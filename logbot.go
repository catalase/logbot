package main

import (
	// "errors"
	"fmt"
	"io"
	// "log"
	// "math"
	"os"
	"reflect"
	// "strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/catalase/bulletin"
)

const (
	logDir = "conversations"
)

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
	channels map[string]bool
	address  string

	*Stream

	closestream chan bool
	onnews      uint32
}

func (bot *Bot) PrivMsg(channel, message string) error {
	return bot.SendMsg("PRIVMSG", channel, message)
}

func (bot *Bot) Broadcast(message string) error {
	for channel := range bot.channels {
		if err := bot.PrivMsg(channel, message); err != nil {
			return err
		}
	}

	return nil
}

func (bot *Bot) Auth() {
	bot.SendMsg("NICK", bot.nick)
	bot.SendMsg("USER", bot.user, "0", "*", bot.realname)
}

func (bot *Bot) Run() {
	bot.Auth()
	bot.Do()
}

func (bot *Bot) Do() {
	msgc := make(chan Msg)
	errc := make(chan error, 1)

	go func() {
	Read:
		msg, err := bot.ReadMsg()
		if err == nil {
			msgc <- msg
			goto Read
		}
		close(msgc)
		errc <- err
	}()

	callbackCache := make(map[string]reflect.Value)
	refbot := reflect.ValueOf(bot)

	for msg := range msgc {
		fmt.Println(msg.Line)

		callbackName := "On" + msg.Command
		callback := callbackCache[callbackName]
		if !callback.IsValid() {
			callback = refbot.MethodByName(callbackName)
		}
		if callback.IsValid() {
			callback.Call([]reflect.Value{
				reflect.ValueOf(msg),
			})
		}
	}

	// err := <-errc
	bot.Close()
	close(bot.closestream)
}

// OnPING responds to PING message.
func (bot *Bot) OnPING(msg Msg) {
	bot.SendMsg("PONG", msg.Args...)
}

func (bot *Bot) Keep() {
	server, _ := Tear(bot.address, ":")
	ticker := time.NewTicker(time.Second * 30)

	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := bot.SendMsg("PING", server); err != nil {
				return
			}
		case <-bot.closestream:
			return
		}
	}
}

func hasNew(old map[string]bool, briefs []bulletin.Brief) bool {
	for _, brief := range briefs {
		if !old[brief.URL()] {
			return true
		}
	}

	return false
}

func makeOld(briefs []bulletin.Brief) map[string]bool {
	old := make(map[string]bool)
	for _, brief := range briefs {
		old[brief.URL()] = true
	}

	return old
}

var KST = time.FixedZone("KST", 32400)

func NowKST() time.Time {
	return time.Now().In(KST)
}

func (bot *Bot) NewNews(old map[string]bool) map[string]bool {
	briefs, err := bulletin.Briefs()
	if err != nil {
		return old
	}

	if !hasNew(old, briefs) {
		return old
	}

	if atomic.LoadUint32(&bot.onnews) == 0 {
		for _, brief := range briefs {
			if url := brief.URL(); !old[url] {
				now := NowKST()
				delta := now.Sub(brief.Time())

				min := uint(delta / time.Minute)
				sec := uint(delta % time.Minute / time.Second)

				if min == 0 {
					bot.Broadcast(fmt.Sprintf("%ds ago - %s", sec, brief.Title()))
				} else {
					bot.Broadcast(fmt.Sprintf("%dm %ds ago - %s", min, sec, brief.Title()))
				}

				bot.Broadcast(brief.URL())
			}
		}
	}

	return makeOld(briefs)
}

func (bot *Bot) News() {
	var old map[string]bool
	ticker := time.NewTicker(5 * time.Second)

	defer ticker.Stop()

Retry:
	if briefs, err := bulletin.Briefs(); err != nil {
		select {
		case <-ticker.C:
			goto Retry
		case <-bot.closestream:
			return
		}
	} else {
		old = makeOld(briefs)
	}

	for {
		select {
		case <-ticker.C:
			old = bot.NewNews(old)
		case <-bot.closestream:
			return
		}
	}
}

// On001
func (bot *Bot) On001(msg Msg) {
	go bot.Keep()
	go bot.News()

	for channel := range bot.channels {
		bot.SendMsg("JOIN", channel)
	}
}

// OnJoin
func (bot *Bot) OnJOIN(msg Msg) {
	nick, channel := msg.Nick(), msg.Args[0]

	if nick == bot.nick {
		bot.channels[channel] = true
	} else {
		bot.PrivMsg(channel, fmt.Sprintf(
			"Welcome %s!", nick))
	}
}

func (bot *Bot) OnPART(msg Msg) {
	nick, channel := msg.Nick(), msg.Args[0]

	if nick == bot.nick {
		delete(bot.channels, channel)
	} else {
		bot.PrivMsg(channel, fmt.Sprintf(
			"Goodbye %s", nick))
	}
}

func (bot *Bot) OnINVITE(msg Msg) {
	invitor, channel := msg.Args[0], msg.Args[1]
	_ = invitor
	bot.SendMsg("JOIN", channel)
}

type ColonAuth int

const (
	COLON_AUTH_ZERO ColonAuth = iota
	COLON_AUTH_OWNER
)

type ColonArgs struct {
	channel string
	which   string
	args    []string
	grant   ColonAuth
	Msg
}

func (bot *Bot) OnPRIVMSG(msg Msg) {
	channel, message := msg.Args[0], msg.Args[1]

	if bot.IsColonMsg(message) {
		bot.ColonMsg(msg)
	}

	_ = channel
}

func (bot *Bot) ColonMsg(msg Msg) {
	channel, message := msg.Args[0], msg.Args[1]

	// Direct message
	if !strings.HasPrefix(channel, "#") {
		bot.PrivMsg(channel, "I deny direct colon message")
		return
	}

	parts := strings.Fields(message[len(":"):])
	which := parts[0]
	args := parts[1:]

	colargs := ColonArgs{
		channel: channel,
		which:   which,
		args:    args,
		grant:   COLON_AUTH_ZERO,
		Msg:     msg,
	}

	bot.ExecColon(colargs)
}

func (*Bot) IsColonMsg(message string) bool {
	return strings.HasPrefix(message, ":")
}

func (bot *Bot) ExecColon(colargs ColonArgs) {
	switch colargs.which {
	case "hello":
		bot.PrivMsg(
			colargs.channel,
			"지옥은 그리 멀지 않은 곳에 있습니다. 전라남도 신안군에 오십시오.",
		)
	case "news":
		if len(colargs.args) > 0 {
			whether := colargs.args[0]
			if whether == "on" {
				atomic.StoreUint32(&bot.onnews, 0)
				bot.PrivMsg(colargs.channel, "자동 뉴스 알림 기능이 켜졌습니다.")
			}
			if whether == "off" {
				atomic.StoreUint32(&bot.onnews, 1)
				bot.PrivMsg(colargs.channel, "자동 뉴스 알림 기능이 꺼졌습니다.")
			}
		}
	}
}

// Mkdir makes a directory specified by path with mode.
// Unlike os.Mkdir, if the directory already exists, a error dose not return.
func Mkdir(path string, mode os.FileMode) error {
	err := os.Mkdir(path, mode)
	if os.IsExist(err) {
		return nil
	}

	return err
}

func main() {
	if err := Mkdir(logDir, os.ModePerm); err != nil {
		fmt.Fprintln(os.Stderr, "cannot make log directory:", err)
		os.Exit(1)
	}

	// server address to connect to
	var address = "irc.ozinger.org:8080"

	// nick to use
	var nick = "비스무트"

	// user
	var user = "ununseptium"

	// realname
	var realname = "하슘"

	// channels to join at the time that the connection establish.
	var channels = map[string]bool{
		"#veritas": true,
		// "#catalase",
		// "#catalase-nut",
		// "#green-net",
		// "#green-pie",
	}

	bot := Bot{
		nick:        nick,
		user:        user,
		realname:    realname,
		channels:    channels,
		closestream: make(chan bool),
	}

	var err error
	if bot.Stream, err = Dial(address); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	bot.Run()
}
