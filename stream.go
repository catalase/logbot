package main

import (
	"bufio"
	"net"
	"strings"
)

// type byteWriter []byte

// func (w *byteWriter) Write(b []byte) (int, error) {
// 	*w = append(*w, b...)
// 	return len(b), nil
// }

type Msg struct {
	Prefix
	command string
	args    []string
	line    string
}

func (msg Msg) String() string {
	return msg.line
}

type Stream struct {
	r *bufio.Reader
	c net.Conn
}

// Msg send command to the server.
func (stream *Stream) Msg(command string, args ...string) error {
	message := []byte(command)

	if len(args) > 0 {
		last := args[len(args)-1]
		if len(args) > 1 {
			for _, arg := range args[:len(args)-1] {
				message = append(message, ' ')
				message = append(message, []byte(arg)...)
			}
		}

		message = append(message, ' ', ':')
		message = append(message, []byte(last)...)
	}

	message = append(message, '\r', '\n')
	_, err := stream.c.Write([]byte(message))

	return err
}

func Chop(str, sep string) (string, string) {
	i := strings.Index(str, sep)
	if i < 0 {
		return str, ""
	}

	return str[:i], str[i+len(sep):]
}

// ReadMsg reads a message as the date of server.
func (stream *Stream) ReadMsg() (Msg, error) {
	byteline, _, err := stream.r.ReadLine()
	if err != nil {
		return Msg{}, err
	}

	var prefix Prefix
	rawline := string(byteline)
	line := rawline

	if line[0] == ':' {
		var prefixraw string
		prefixraw, line = Chop(line, " ")
		prefix = Prefix(prefixraw)
	}

	line, tr := Chop(line, " :")
	command, line := Chop(line, " ")

	var arg string
	var args []string
	for len(line) > 0 {
		arg, line = Chop(line, " ")
		args = append(args, arg)
	}
	args = append(args, tr)

	msg := Msg{
		prefix,
		command,
		args,
		rawline,
	}

	return msg, nil
}

// Close closes the connection to server.
func (stream *Stream) Close() (err error) {
	if stream.c != nil {
		err = stream.c.Close()
	}

	stream.c = nil
	stream.r = nil

	return
}

func NewStream(address string) (*Stream, error) {
	c, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	r := bufio.NewReader(c)
	stream := &Stream{
		r: r,
		c: c,
	}

	return stream, nil
}

// https://en.wikipedia.org/wiki/Client-to-client_streamcol
// CTCP <target> <command> <arguments>
// a CTCP is the message that starts with \x01
func IsCTCP(message string) bool {
	return strings.HasPrefix(message, "\x01")
}
