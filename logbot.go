package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var loggerMap = make(LoggerMap)

type LoggerMap map[string]*os.File

func (loggerMap LoggerMap) Get(channel string) io.Writer {
	w, ok := loggerMap[channel]
	if !ok {
		return ioutil.Discard
	}

	return w
}

func (loggerMap LoggerMap) Set(channel, path string) error {
	w, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE, 0200)
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
	voice      chan [2]string
	closevoice chan bool
}

func (bot *Bot) Voice(channel, message string) {
	voice := [2]string{
		channel,
		message,
	}

	select {
	case bot.voice <- voice:
	case <-bot.closevoice:
	}
}

func (bot *Bot) Msg(command string, args ...string) error {
	bot.OnMsg(command, args...)
	return bot.Stream.Msg(command, args...)
}

func (bot *Bot) Run() error {
	bot.voice = make(chan [2]string)
	bot.closevoice = make(chan bool)

	go func() {
		var voices [][2]string
		var timer <-chan time.Time

		for {
			select {
			case voice := <-bot.voice:
				if len(voices) == 0 {
					if timer == nil {
						c := make(chan time.Time, 1)
						c <- time.Now()
						timer = c
					} else {
						timer = time.After(time.Second)
					}
				}
				voices = append(voices, voice)
			case <-timer:
				if len(voices) > 0 {
					voice := voices[0]
					voices = voices[1:]
					bot.Msg("PRIVMSG", voice[0], voice[1])
					if len(voices) > 0 {
						timer = time.After(time.Second)
					}
				}
			case <-bot.closevoice:
				return
			}
		}
	}()

	bot.Auth()

	for {
		msg, err := bot.ReadMsg()
		if err != nil {
			close(bot.closevoice)
			bot.Close()
			return err
		}

		bot.OnRead(msg)

		callback := reflect.ValueOf(bot).MethodByName(
			"On" + strings.ToUpper(msg.command))

		if callback.IsValid() {
			callback.Call([]reflect.Value{
				reflect.ValueOf(msg),
			})
		}
	}
}

func (bot *Bot) Auth() {
	bot.Msg("NICK", bot.nick)
	bot.Msg("USER", bot.user, "0", "*", bot.realname)
}

func (bot *Bot) OnPING(msg Msg) {
	bot.Msg("PONG", msg.args...)
}

func (bot *Bot) OnMsg(command string, args ...string) {
	line := BuildMsg(command, args)
	fmt.Println("<-", line)
	fmt.Fprintln(loggerMap.Get("!"), "<-", line)
}

func (bot *Bot) OnRead(msg Msg) {
	fmt.Println("->", msg)
	fmt.Fprintln(loggerMap.Get("!"), "->", msg.line)
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
	fmt.Fprintln(loggerMap.Get(channel), msg.line)

	if msg.Nick() != bot.nick {
		if strings.HasPrefix(text, ":") {
			bot.OnColon(msg)
			return
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
			"다음 명령어가 사용 가능합니다: :ln, :log, :ver, :전역",
		)

	case "ln":
		bot.OnLn(channel, args)

	case "log":
		bot.OnLog(channel, args)

	case "ver":
		bot.Voice(channel, "running on "+runtime.GOOS)

	case "전역":
		bot.OnEsc(channel, args)

	case "##":
		if channel == bot.nick {
			bot.OnSig(channel, args, msg)
		}
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
	bot.Voice(channel, "귀찮아서 안만듬 전역 날짜 계산은 내가 만든 사이트 가서 보셈")
}

func (bot *Bot) OnSig(channel string, args []string, msg Msg) {
	if len(args) > 0 {
		bot.Msg(args[0], args[1:]...)
	}
}

func tidy() {
	for _, logger := range loggerMap {
		logger.Close()
	}
}

func main() {
	var err error

	if err = os.Mkdir(
		"logging",
		os.ModePerm,
	); err != nil && !os.IsExist(err) {
		log.Print(err)
		return
	}

	if err := os.Chdir("logging"); err != nil {
		log.Print(err)
		return
	}

	address := "kanade.irc.ozinger.org:8080"
	bot := Bot{
		nick:     "비스무트",
		user:     "ununseptium",
		realname: "하슘",
		channels: []string{
			"#catalase",
			"#catalase-nut",
			"#green-net",
			"#green-pie",
		},
	}

	for _, channel := range append(
		bot.channels,
		"!",
		bot.nick,
	) {
		if err = loggerMap.Set(
			channel,
			channel+".txt",
		); err != nil {
			log.Print(err)
			tidy()
			return
		}
	}

	for {
		bot.Stream, err = NewStream(address)
		if err != nil {
			log.Printf("cannot connect to irc server %s becuase %s", address, err)
			time.Sleep(30 * time.Second)
			continue
		}

		log.Print(bot.Run())
		log.Print("restart bot in 1 minute")
		time.Sleep(time.Minute)
	}
}
