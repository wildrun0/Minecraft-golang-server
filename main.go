package main

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/robfig/cron/v3"

	"github.com/Tnze/go-mc/net"

	"github.com/Tnze/go-mc/chat"
	ru_ru "github.com/Tnze/go-mc/data/lang/ru-ru"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

const ProtocolVersion = 578

// Packet IDs
const (
	PlayerPositionAndLookClientbound = 0x36
	JoinGame                         = 0x26
)

type server_settings_type struct {
	ip_addr       string
	port_addr     string
	difficulty    string
	server_lang   string
	motd          string
	view_distance int
	max_players   int
}

var server_settings server_settings_type

func server_settings_checker(filename string) {
	file, err := os.Open(filename)
	defer file.Close()
	if err != nil {
		if file, err := os.Create(filename); err != nil {
			log.Print("UNABLE TO CREATE A CONFIGURATION FILE")
		} else {
			server_default_settings := "ip_addr=127.0.0.1\nport_addr=25565\ndifficulty=easy\nserver_lang=en\nmotd=A Golang Minecraft Server\nview_distance=16\nmax_players=20"
			_, _ = file.WriteString(server_default_settings)
		}
		panic(string("File " + filename + " dont exist. Creating a new one!\n(Now you should open file and edit him as you like)"))
	} else {
		lines, _ := ioutil.ReadAll(file)
		line_strings := strings.Split(string(lines), "\n")
		for i := 0; i < len(line_strings); i++ {
			line := line_strings[i]
			switch strings.Split(string(line), "=")[0] {
			case "ip_addr":
				server_settings.ip_addr = strings.Split(string(line), "=")[1]
			case "port_addr":
				server_settings.port_addr = strings.Split(string(line), "=")[1]
			case "difficulty":
				server_settings.difficulty = strings.Split(string(line), "=")[1]
			case "server_lang":
				server_settings.server_lang = strings.Split(string(line), "=")[1]
			case "motd":
				server_settings.motd = strings.Split(string(line), "=")[1]
			case "view_distance":
				server_settings.view_distance, _ = strconv.Atoi(strings.Split(string(line), "=")[1])
			case "max_players":
				server_settings.max_players, _ = strconv.Atoi(strings.Split(string(line), "=")[1])
			}
		}
	}
}

func main() {
	server_settings_checker("server_settings.txt")
	l, err := net.ListenMC(server_settings.ip_addr + ":" + server_settings.port_addr)
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

//Store all connections
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
func send_public_chat(message string, announce_type byte, color string) {
	for i := 0; i < len(players_conns); i++ {
		if err := players_conns[i].WritePacket(pk.Marshal(0x0F, chat.Message{Text: message, Color: color}, pk.Byte(announce_type))); err != nil {
			log.Print(err)
			send_error(players_conns[i], "Error just happend")
		}
	}
}

type player_tab_config struct {
	UUID     pk.UUID
	Name     pk.String
	nop      pk.VarInt
	gamemode pk.VarInt
	ping     pk.VarInt
	hdn      pk.Boolean
}

// cringe department
func clean_player(r net.Conn, player player_tab_config) {
	for i, v := range players_conns {
		if v == r {
			players_conns[i] = players_conns[len(players_conns)-1]
			players_conns = players_conns[:len(players_conns)-1]
		}
	}
	for i, v := range players_online {
		if v == player {
			tab_remove_user(players_online[i].UUID)
			players_online[i] = players_online[len(players_online)-1]
			players_online = players_online[:len(players_online)-1]
		}
	}
}

var players_online []player_tab_config

func tab_remove_user(UUID pk.UUID) {
	for i := 0; i < len(players_conns); i++ {
		_ = players_conns[i].WritePacket(pk.Marshal(0x34,
			pk.VarInt(4), // action -> remove player
			pk.VarInt(1),
			UUID))
	}
}

func tab_update_users(conn net.Conn) {
	for i := 0; i < len(players_online); i++ {
		_ = conn.WritePacket(pk.Marshal(0x34,
			pk.VarInt(0), //action,
			pk.VarInt(1), //Number Of Players,
			players_online[i].UUID, players_online[i].Name, players_online[i].nop, players_online[i].gamemode, players_online[i].ping, players_online[i].hdn))
	}
}
func spawn_player(conn net.Conn, eid pk.VarInt, uuid pk.UUID, x, y, z pk.Double, yaw, pitch pk.Angle) {
	_ = conn.WritePacket(pk.Marshal(0x05,
		eid,
		uuid,
		x, y, z, // XYZ
		yaw, pitch))
}
func handlePlaying(conn net.Conn, protocol int32) {
	c := cron.New()
	_, _ = c.AddFunc("@every 25s", func() {
		rand_float := rand.Uint64()
		if err := conn.WritePacket(pk.Marshal(0x21, pk.Long(rand_float))); err != nil {
			log.Print("Error when sending KEEP ALIVE PACKET", err)
		}
	})
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

	if err := joinGame(conn, info.Name); err != nil {
		log.Print("Login failed on joinGame")
		return
	}
	if err := conn.WritePacket(pk.Marshal(PlayerPositionAndLookClientbound,
		// https://wiki.vg/index.php?title=Protocol&oldid=16067#Player_Position_And_Look_.28clientbound.29
		pk.Double(0), pk.Double(0), pk.Double(0), // XYZ
		pk.Float(0), pk.Float(0), // Yaw Pitch
		pk.Byte(0),   // flag
		pk.VarInt(0), // TP ID
	)); err != nil {
		log.Print("Login failed on sending PlayerPositionAndLookClientbound")
		return
	}
	//https://wiki.vg/index.php?title=Protocol&oldid=16067#Player_Info
	player_to_tab := player_tab_config{pk.UUID(info.UUID), pk.String(info.Name), pk.VarInt(0), pk.VarInt(1), pk.VarInt(100), pk.Boolean(false)}

	//Sending TAB MENU with all users to the current player
	tab_update_users(conn)
	//Adding current player to the list of players
	players_online = append(players_online, player_to_tab)

	//Adding current player to the others TAB Menu
	for i := 0; i < len(players_conns); i++ {
		_ = players_conns[i].WritePacket(pk.Marshal(0x34,
			pk.VarInt(0), //action,
			pk.VarInt(1), //Number Of Players,
			pk.UUID(info.UUID), pk.String(info.Name), pk.VarInt(0), pk.VarInt(1), pk.VarInt(100), pk.Boolean(false)))
	}
	// Just for block this goroutine. Keep the connection
	chat.SetLanguage(ru_ru.Map) //not sure if this needed
	c.Start()
	player_joined_message := info.Name + " has joined the server"
	log.Print(player_joined_message + " [EID:" + strconv.Itoa(info.Eid) + "]")
	send_public_chat(player_joined_message, 1, "yellow")
	var p pk.Packet
	for {
		if err := conn.ReadPacket(&p); err != nil {
			player_left_message := info.Name + " has left the server"
			send_public_chat(player_left_message, 1, "yellow")
			log.Print(player_left_message)
			log.Printf("ReadPacket error: %v", err)
			c.Stop()
			clean_player(conn, player_to_tab)
			break
		} else {
			switch p.ID {
			case 0x03: //CHAT MESSAGE
				chat_message := string(p.Data[1:]) // removing junk byte (probably junk, idk maxim ya hz)
				send_public_chat(info.Name+": "+chat_message, 0, "white")
				if chat_message == "/disconnect" {
					disconnect(conn, "REASON")
				} else if strings.Contains(chat_message, "/difficulty") {
					if splitted_string := strings.Split(chat_message, " "); len(splitted_string) > 1 {
						diff_type, _ := strconv.Atoi(strings.Split(chat_message, " ")[1])
						difficulty(conn, diff_type)
					} else {
						send_error(conn, "Wrong command! Use /difficulty [1-3]")
					}
				} else if chat_message == "/test" {
					fmt.Println(players_conns)
				}
			case 0x0F: //KEEP ALIVE ANSWER, NOTHING TO DO HERE
			case 0x05: //CLIENT SETTINGS, IGNORING THIS
			case 0x0B: //PLUGIN MESSAGE, IGNORING this
			case 0x12: //Player Position And Look
				var x, y, z pk.Double
				var yaw, pitch pk.Angle
				var flag pk.Byte
				var on_ground pk.Boolean
				_ = p.Scan(&x, &y, &z, &yaw, &pitch, &flag, &on_ground)
				fmt.Println(x, y, z, yaw, pitch, on_ground)
			case 0x2A: //Player arm animation
				var arm_swing pk.VarInt
				_ = p.Scan(&arm_swing)
				conn.WritePacket(pk.Marshal(0x06, pk.VarInt(players_eid[info.Name]), pk.UnsignedByte(arm_swing)))
			default:
				fmt.Println(p)
			}
		}
	}
}

type PlayerInfo struct {
	Name    string
	UUID    uuid.UUID
	OPLevel int
	Eid     int
}

var players_eid = make(map[string]int)

// acceptLogin check player's account
func acceptLogin(conn net.Conn) (info PlayerInfo, err error) {
	//login start
	var p pk.Packet
	err = conn.ReadPacket(&p)
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
	if len(players_eid) == 0 {
		info.Eid = 0
		players_eid[info.Name] = 0
	} else {
		for i := 0; i < len(players_eid); i++ {
			info.Eid = i
			players_eid[info.Name] = i
		}
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
	if err = conn.ReadPacket(&p); err != nil {
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

func joinGame(conn net.Conn, Name string) error {
	return conn.WritePacket(pk.Marshal(JoinGame,
		pk.Int(players_eid[Name]), // EntityID
		pk.UnsignedByte(0),        // Gamemode
		pk.Int(0),                 // Dimension
		pk.Long(0),                // HashedSeed
		pk.UnsignedByte(server_settings.max_players), // MaxPlayer
		pk.String("default"),                         // LevelType
		pk.VarInt(server_settings.view_distance),     // View Distance
		pk.Boolean(false),                            // Reduced Debug Info
		pk.Boolean(true),                             // Enable respawn screen
	))
}
