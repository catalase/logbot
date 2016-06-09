package main

import (
	"bufio"
	"net"
	"strings"
)

// Msg represents a message used in IRC.
type Msg struct {
	Prefix
	Command string
	Args    []string
	Line    string
}

// func (message Msg) First() string {
// 	if len(message.Args) > 0 {
// 		return message.Args[0]
// 	}
// 	return ""
// }

// Tr returnes the trailer of the message (i.e. last element of Args)
func (message Msg) Tr() string {
	if i := len(message.Args); i > 0 {
		return message.Args[i-1]
	}
	return ""
}

// String returnes raw string representation.
func (message Msg) String() string {
	return message.Line
}

type Stream struct {
	r *bufio.Reader
	c net.Conn
}

// SendMsg sends message to the server.
func (stream *Stream) SendMsg(command string, args ...string) error {
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

// ReadMsg reads a message as the date of server.
func (stream *Stream) ReadMsg() (Msg, error) {
	byteLine, _, err := stream.r.ReadLine()
	if err != nil {
		return Msg{}, err
	}

	var prefix Prefix
	line := string(byteLine)

	if len(line) > 0 {
		if line[0] == ':' {
			var left string
			left, line = Tear(line, " ")
			prefix = Prefix(left)
		}
	}

	line, tr := Tear(line, " :")
	command, line := Tear(line, " ")

	var (
		arg  string
		args []string
	)

	for len(line) > 0 {
		arg, line = Tear(line, " ")
		args = append(args, arg)
	}

	if len(tr) > 0 {
		args = append(args, tr)
	}

	msg := Msg{
		prefix,
		strings.ToUpper(command),
		args,
		string(byteLine),
	}

	return msg, nil
}

// Close closes all resource associated with the stream.
func (stream *Stream) Close() (err error) {
	if stream.c != nil {
		err = stream.c.Close()
	}

	stream.c = nil
	stream.r = nil

	return
}

// Tear rips apart str by sep into two parts.
func Tear(str, sep string) (string, string) {
	i := strings.Index(str, sep)
	if i < 0 {
		return str, ""
	}

	return str[:i], str[i+len(sep):]
}

// Dial creates new Stream.
func Dial(address string) (*Stream, error) {
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
