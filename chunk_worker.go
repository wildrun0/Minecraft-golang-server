package main

import (
	"bytes"
	"unsafe"

	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/save"
	"github.com/Tnze/go-mc/save/region"
)

func ExampleColumn_send(conn net.Conn, path string) {
	r, err := region.Open(path)
	if err != nil {
		panic(err)
	}
	chunkPos := [2]int{0, 0}
	data, err := r.ReadSector(chunkPos[0], chunkPos[1])
	if err != nil {
		panic(err)
	}

	var c save.Column
	if err := c.Load(data); err != nil {
		panic(err)
	}

	var buf bytes.Buffer
	var PrimaryBitMask pk.VarInt
	for _, v := range c.Level.Sections {
		if int8(v.Y) >= 0 && int8(v.Y) < 16 {
			PrimaryBitMask |= 1 << v.Y

			bpb := len(v.BlockStates) * 64 / (16 * 16 * 16)
			hasPalette := pk.Boolean(bpb >= 9)
			paletteLength := pk.VarInt(len(v.Palette))
			dataArrayLength := pk.VarInt(len(v.BlockStates))
			dataArray := (*[]pk.Long)(unsafe.Pointer(&v.BlockStates))
			_, err := pk.Tuple{
				pk.Short(0),          // Block count
				pk.UnsignedByte(bpb), // Bits Per Block
				hasPalette, pk.Opt{
					Has: &hasPalette,
					Field: pk.Tuple{
						paletteLength, pk.Ary{
							Len: &paletteLength,
							Ary: nil, // TODO: We need translate v.Palette (with type of []Block) to state ID
						},
					},
				}, // Palette
				dataArrayLength, pk.Ary{
					Len: &dataArrayLength,
					Ary: dataArray,
				}, // Data Array
			}.WriteTo(&buf)
			if err != nil {
				panic(err)
			}
		}
	}

	size := pk.VarInt(buf.Len())
	bal := pk.VarInt(len(c.Level.Biomes))
	chunk_ready := pk.Marshal(
		0x22,
		pk.Int(chunkPos[0]),        // Chunk X
		pk.Int(chunkPos[1]),        // Chunk Y
		pk.Boolean(true),           // Full chunk
		PrimaryBitMask,             // PrimaryBitMask
		pk.NBT(c.Level.Heightmaps), // Heightmaps
		bal, pk.Ary{
			Len: bal,                                              // Biomes array length
			Ary: *(*[]pk.VarInt)(unsafe.Pointer(&c.Level.Biomes)), // Biomes
		},
		size, pk.Ary{
			Len: size,                      // Size
			Ary: pk.ByteArray(buf.Bytes()), // Data
		},
		pk.VarInt(0), // Block entities array length
	)
	conn.WritePacket(chunk_ready)
}
