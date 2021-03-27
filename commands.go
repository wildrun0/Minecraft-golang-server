package main

import (
	"log"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
)

func send_private_chat(conn net.Conn, message string, color string) {
	if err := conn.WritePacket(pk.Marshal(0x0F, chat.Message{Text: message, Color: color}, pk.Byte(0))); err != nil {
		log.Print(err)
	}
}
func send_error(conn net.Conn, erro string) {
	send_private_chat(conn, erro, "red")
}

func disconnect(conn net.Conn, reason string) {
	conn.WritePacket(pk.Marshal(0x1B, chat.Text(reason)))
}
func difficulty(conn net.Conn, diff_type int) {
	if diff_type < 0 || diff_type > 3 {
		send_error(conn, "Wrong command! Use /difficulty [1-3]")
	} else {
		conn.WritePacket(pk.Marshal(0x0E, pk.Byte(diff_type), pk.Boolean(true)))
		send_public_chat(string("Сложность изменена на: "+difficulties[diff_type]), 1, "yellow")
	}
}
