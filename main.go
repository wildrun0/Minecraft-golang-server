package main

import (
	"crypto/md5"
	"strings"
	"strconv"
	"fmt"
	"log"
	"math/rand"
	// "os"

	"github.com/Tnze/go-mc/net"

	"github.com/Tnze/go-mc/chat"
	ru_ru "github.com/Tnze/go-mc/data/lang/ru-ru"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

const ProtocolVersion = 578
const MaxPlayer = 100

// Packet IDs
const (
	PlayerPositionAndLookClientbound = 0x36
	JoinGame                         = 0x26
)

var (
	ip_addr   = "127.0.0.1"
	port_addr = "25565"
)

func main() {
	l, err := net.ListenMC(ip_addr + ":" + port_addr)
	if err != nil {
		log.Fatalf("Listen error: %v", err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatalf("Accept error: %v", err)
		}
		go acceptConn(conn)
	}
}

// NameToUUID return the UUID from player name in offline mode
func NameToUUID(name string) uuid.UUID {
	var version = 3
	h := md5.New()
	h.Reset()
	h.Write([]byte("OfflinePlayer:" + name))
	s := h.Sum(nil)
	var id uuid.UUID
	copy(id[:], s)
	id[6] = (id[6] & 0x0f) | uint8((version&0xf)<<4)
	id[8] = (id[8] & 0x3f) | 0x80 // RFC 4122 variant
	return id
}
var players_conns []net.Conn
func acceptConn(conn net.Conn) {
	defer conn.Close()
	// handshake
	protocol, intention, err := handshake(conn)
	if err != nil {
		log.Printf("Handshake error: %v", err)
		return
	}

	switch intention {
	default: //unknown error
		log.Printf("Unknown handshake intention: %v", intention)
	case 1: //for status
		acceptListPing(conn)
	case 2: //for login
		players_conns = append(players_conns, conn)
		handlePlaying(conn, protocol)
	}
}
func send_public_chat(message string, announce_type byte){
	for i:=0; i < len(players_conns); i++{
		players_conns[i].WritePacket(pk.Marshal(0x0F, chat.Text(message), pk.Byte(announce_type)))
	}
}
func send_private_chat(conn net.Conn, message string){
	conn.WritePacket(pk.Marshal(0x0F, chat.Text(message), pk.Byte(0)))
}
var difficulties = [4]string{"Мирная","Нормальная","Сложная","Хардкор"}
func handlePlaying(conn net.Conn, protocol int32) {
	// login, get player info
	info, err := acceptLogin(conn)
	if err != nil {
		log.Print("Login failed")
		return
	}

	// Write LoginSuccess packet

	if err = loginSuccess(conn, info.Name, info.UUID); err != nil {
		log.Print("Login failed on success")
		return
	}

	if err := joinGame(conn); err != nil {
		log.Print("Login failed on joinGame")
		return
	}
	if err := conn.WritePacket(pk.Marshal(PlayerPositionAndLookClientbound,
		// https://wiki.vg/index.php?title=Protocol&oldid=16067#Player_Position_And_Look_.28clientbound.29
		pk.Double(10), pk.Double(0), pk.Double(10), // XYZ
		pk.Float(0), pk.Float(0), // Yaw Pitch
		pk.Byte(0),   // flag
		pk.VarInt(0), // TP ID
	)); err != nil {
		log.Print("Login failed on sending PlayerPositionAndLookClientbound")
		return
	}
	// Just for block this goroutine. Keep the connection
	chat.SetLanguage(ru_ru.Map)
	for {
		if p, err := conn.ReadPacket(); err != nil {
			log.Printf("ReadPacket error: %v", err)
			break
		} else {
			switch p.ID {
			case 0x03: //CHAT MESSAGE
				chat_message := string(p.Data[1:]) // removing junk byte (probably junk, idk maxim ya hz)
				send_public_chat(info.Name+": "+chat_message, 0)
				if chat_message == "/disconnect" {
					conn.WritePacket(pk.Marshal(0x1B, chat.Text("ПОСОСИ У У У")))
				} else if strings.Contains(chat_message, "/difficulty"){
					diff_type, _ := strconv.Atoi(strings.Split(chat_message, " ")[1])
					conn.WritePacket(pk.Marshal(0x0E, pk.Byte(0), pk.Boolean(false)))
					send_public_chat(string("Сложность была изменена на "+difficulties[diff_type]), 1)
					// conn.WritePacket(pk.Marshal(0x0F, chat.Text("Сложность была изменена"), pk.Byte(1)))
				} else if chat_message == "/test"{
					fmt.Println(players_conns)
				}
			case 0xF:
			default:
				fmt.Println(p.ID)
			}
		}
		conn.WritePacket(pk.Marshal(0x21, pk.Long(rand.Uint64())))
	}
}

type PlayerInfo struct {
	Name    string
	UUID    uuid.UUID
	OPLevel int
}

// acceptLogin check player's account
func acceptLogin(conn net.Conn) (info PlayerInfo, err error) {
	//login start
	var p pk.Packet
	p, err = conn.ReadPacket()
	if err != nil {
		return
	}

	err = p.Scan((*pk.String)(&info.Name)) //decode username as pk.String
	if err != nil {
		return
	}

	//auth
	const OnlineMode = false
	if OnlineMode {
		log.Panic("Not Implement")
	} else {
		// offline-mode UUID
		info.UUID = NameToUUID(info.Name)
	}

	return
}

// handshake receive and parse Handshake packet
func handshake(conn net.Conn) (protocol, intention int32, err error) {
	var (
		p                   pk.Packet
		Protocol, Intention pk.VarInt
		ServerAddress       pk.String        // ignored
		ServerPort          pk.UnsignedShort // ignored
	)
	// receive handshake packet
	if p, err = conn.ReadPacket(); err != nil {
		return
	}
	err = p.Scan(&Protocol, &ServerAddress, &ServerPort, &Intention)
	return int32(Protocol), int32(Intention), err
}

// loginSuccess send LoginSuccess packet to client
func loginSuccess(conn net.Conn, name string, uuid uuid.UUID) error {
	return conn.WritePacket(pk.Marshal(0x02,
		pk.String(uuid.String()), //uuid as string with hyphens
		pk.String(name),
	))
}

func joinGame(conn net.Conn) error {
	return conn.WritePacket(pk.Marshal(JoinGame,
		pk.Int(0),                  // EntityID
		pk.UnsignedByte(1),         // Gamemode
		pk.Int(0),                  // Dimension
		pk.Long(0),                 // HashedSeed
		pk.UnsignedByte(MaxPlayer), // MaxPlayer
		pk.String("default"),       // LevelType
		pk.VarInt(15),              // View Distance
		pk.Boolean(false),          // Reduced Debug Info
		pk.Boolean(true),           // Enable respawn screen
	))
}
