package main

import (
	"fmt"
	"io"
	// "io/ioutil"
	"log"
	"math"
	"os"
	"reflect"
	// "runtime"
	"strconv"
	"strings"
	"time"
	// "sync"
)

const logsdir = "logs"

var loggerMap = make(LoggerMap)

type LoggerMap map[string]*os.File

func (loggerMap LoggerMap) Get(channel string) io.Writer {
	return loggerMap[channel]
}

func (loggerMap LoggerMap) Set(channel, path string) error {
	w, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE, 0777)
	if err == nil {
		loggerMap[channel] = w
	}

	return err
}

func BuildMsg(command string, args []string) string {
	if len(args) == 0 {
		return command
	}

	if len(args) == 1 {
		return command + " :" + args[0]
	}

	o := strings.Join(args[:len(args)-1], " ")
	o += " :" + args[len(args)-1]

	return command + " " + o
}

type Bot struct {
	nick     string
	user     string
	realname string
	channels []string

	*Stream

	// a element at index 0 = channel (or target)
	// a element of index 1 = text to be sent to the channel (or target)
	voice       chan Voice
	closestream chan bool
}

type Voice struct {
	channel string
	message string
}

func (bot *Bot) Voice(channel, message string) {
	select {
	case bot.voice <- Voice{
		channel,
		message,
	}:
	case <-bot.closestream:
	}
}

func (bot *Bot) Msg(command string, args ...string) error {
	bot.OnMsg(command, args...)
	return bot.Stream.Msg(command, args...)
}

func (bot *Bot) Run() error {
	bot.voice = make(chan Voice)
	bot.closestream = make(chan bool)

	go func() {
		var attend = make(map[string]chan Voice)

		for {
			select {
			case voice := <-bot.voice:
				line, ok := attend[voice.channel]
				if !ok {
					line = make(chan Voice)
					attend[voice.channel] = line
					go func(channel string, line chan Voice) {
						var voices []Voice
						var tick <-chan time.Time

						ticker := time.NewTicker(time.Second)

						defer func() {
							ticker.Stop()
							delete(attend, channel)
						}()

						for {
							select {
							case voice := <-line:
								if len(voices) == 0 {
									tick = ticker.C
								}

								voices = append(voices, voice)

							case <-tick:
								voice := voices[0]
								voices = voices[1:]
								if len(voices) == 0 {
									tick = nil
								}

								bot.Msg(
									"PRIVMSG",
									voice.channel,
									voice.message,
								)

							case <-bot.closestream:
								return
							}
						}
					}(voice.channel, line)
				}

				line <- voice
			}
		}
	}()

	// go func() {
	// 	var voices [][2]string
	// 	var timer <-chan time.Time

	// 	for {
	// 		select {
	// 		case voice := <-bot.voice:
	// 			if len(voices) == 0 {
	// 				if timer == nil {
	// 					c := make(chan time.Time, 1)
	// 					c <- time.Now()
	// 					timer = c
	// 				} else {
	// 					timer = time.After(time.Second)
	// 				}
	// 			}
	// 			voices = append(voices, voice)
	// 		case <-timer:
	// 			if len(voices) > 0 {
	// 				voice := voices[0]
	// 				voices = voices[1:]
	// 				bot.Msg("PRIVMSG", voice[0], voice[1])
	// 				if len(voices) > 0 {
	// 					timer = time.After(time.Second)
	// 				}
	// 			}
	// 		case <-bot.closestream:
	// 			return
	// 		}
	// 	}
	// }()

	go func() {
		ticker := time.NewTicker(30 * time.Second)

		for {
			select {
			case <-ticker.C:
				bot.Msg("PING", "kanade.irc.ozinger.org")
			case <-bot.closestream:
				ticker.Stop()
				return
			}
		}
	}()

	bot.Msg("NICK", bot.nick)
	bot.Msg("USER", bot.user, "0", "*", bot.realname)

	for {
		msg, err := bot.ReadMsg()
		if err != nil {
			close(bot.closestream)
			bot.Close()
			return err
		}

		fmt.Println("->", msg)
		fmt.Fprintln(loggerMap.Get("mid"), "->", msg.line)

		callback := reflect.ValueOf(bot).MethodByName(
			"On" + strings.ToUpper(msg.command))

		if callback.IsValid() {
			callback.Call([]reflect.Value{
				reflect.ValueOf(msg),
			})
		}
	}
}

func (bot *Bot) OnPING(msg Msg) {
	bot.Msg("PONG", msg.args...)
}

func (bot *Bot) OnMsg(command string, args ...string) {
	line := BuildMsg(command, args)
	fmt.Println("<-", line)
	fmt.Fprintln(loggerMap.Get("mid"), "<-", line)
}

func (bot *Bot) On001(msg Msg) {
	for _, channel := range bot.channels {
		bot.Msg("JOIN", channel)
	}
}

func (bot *Bot) OnJOIN(msg Msg) {
	if msg.Nick() != bot.nick {
		bot.Voice(
			msg.args[0],
			msg.Nick()+", 이 채널은 로그봇에 의하여 모든 대화가 기록됩니다."+
				"로그봇이므로 로그 계산도 가능합니다. 자세한 것은 :logbot 을 외쳐주세요.",
		)
	}
}

func (bot *Bot) OnPRIVMSG(msg Msg) {
	channel, text := msg.args[0], msg.args[1]
	if logger := loggerMap.Get(channel); logger != nil {
		fmt.Fprintln(logger, msg.line)
	}

	if msg.Nick() != bot.nick {
		if strings.HasPrefix(channel, "#") {
			if strings.HasPrefix(text, ":") {
				bot.OnColon(msg)
				return
			}
		}
	}
}

func (bot *Bot) OnColon(msg Msg) {
	channel, text := msg.args[0], msg.args[1]
	command, argline := Chop(
		text[len(":"):],
		" ",
	)
	args := strings.Fields(argline)

	switch command {
	case "logbot":
		bot.Voice(
			channel,
			"다음 명령어가 사용 가능합니다: :ln, :log, :ver, :전역, :쿠키런크리스탈수",
		)

	case "ln":
		bot.OnLn(channel, args)

	case "log":
		bot.OnLog(channel, args)

	case "ver":
		bot.Voice(channel, "logbot 0.1")

	case "쿠키런크리스탈수":
		bot.Voice(channel, "를 전화해서 물어보세요.")
		// bot.Voice(channel, "the number of crystals you have in 쿠키런 is a neighborhood of fifteen hundred.")

	case "전역":
		bot.OnEsc(channel, args)

		// case "*":
		// 	if channel == bot.nick {
		// 		bot.OnSig(channel, args, msg)
		// 	}
	}
}

func (bot *Bot) OnLn(channel string, args []string) {
	if len(args) != 1 {
		bot.Voice(channel, fmt.Sprintf(
			":ln expected 1 argument, but given %d arguments", len(args)))
		return
	}

	x, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return
	}

	bot.Voice(channel, strconv.FormatFloat(
		math.Log(x), 'f', 32, 64))
}

func (bot *Bot) OnLog(channel string, args []string) {
	if len(args) != 2 {
		bot.Voice(channel, fmt.Sprintf(
			":log expected 2 arguments, but given %d arguments", len(args)))
		return
	}

	x, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return
	}

	y, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return
	}

	bot.Voice(channel, strconv.FormatFloat(
		math.Log(x)/math.Log(y), 'f', 32, 64))
}

func (bot *Bot) OnEsc(channel string, args []string) {
	bot.Voice(channel, "http://catalase.github.io/ 가서 확인하세요.")
}

func (bot *Bot) OnSig(channel string, args []string, msg Msg) {
	if len(args) > 0 {
		bot.Msg(args[0], args[1:]...)
	}
}

func Mkdir(path string, mode os.FileMode) error {
	err := os.Mkdir(path, mode)
	if os.IsExist(err) {
		return nil
	}

	return err
}

func main() {
	if err := Mkdir(logsdir, os.ModePerm); err != nil {
		fmt.Println("cannot mkdir logs with permission 0777:", err)
		return
	}

	if err := os.Chdir(logsdir); err != nil {
		fmt.Println("cannot change root directory to", logsdir)
		return
	}

	defer func() {
		for _, o := range loggerMap {
			o.Close()
		}
	}()

	var address = "kanade.irc.ozinger.org:8080"
	var nick = "비스무트"
	var user = "ununseptium"
	var realname = "하슘"
	var channels = []string{
		"#catalase",
		"#catalase-nut",
		"#green-net",
		"#green-pie",
	}

	for _, channel := range append(
		channels,
		"mid",
		nick,
	) {
		path := channel + ".txt"
		if err := loggerMap.Set(channel, path); err != nil {
			fmt.Println("cannot open logger for channel", channel,
				"into which conversation or exchange in the channel are written:", err)
			return
		}
	}

	bot := Bot{
		nick:     nick,
		user:     user,
		realname: realname,
		channels: channels,
	}

	for {
		var err error
		bot.Stream, err = NewStream("kanade.irc.ozinger.org:8080")
		if err != nil {
			log.Println(address, "cannot be connected to:", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println(bot.Run())
		time.Sleep(time.Second)
	}
}
