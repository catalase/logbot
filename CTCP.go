package main

import (
	"strings"
)

// IsCTCP returns whether the message is a CTCP message or not and
// , if so, the tag and the text contained in the CTCP message.
//
// BNF for a CTCP message is
//
//   <text>  ::= <delim> <tag> [<SPACE> <message>] <delim>
//   <delim> ::= 0x01
//
// Reference for it.
//   https://en.wikipedia.org/wiki/Client-to-client_protocol
//   https://godoc.org/gopkg.in/sorcix/irc.v1/ctcp#Decode
//
func IsCTCP(message string) (tag string, text string, isCTCP bool) {
	isCTCP = strings.HasPrefix(message, "\x01") && strings.HasSuffix(message, "\x01")
	if isCTCP {
		tag, text = Tear(message[1:len(message)-2], " ")
	}
	return
}
