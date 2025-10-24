package dispatcher

import (
	"encoding/hex"
	"fmt"
	"net"

	"codec-svr/internal/codec"
)

func ProcessIncoming(conn net.Conn, data []byte) {
	// Mostrar datos crudos
	rawHex := hex.EncodeToString(data)
	fmt.Printf("🔹 RAW HEX (%d bytes): %s\n", len(data), rawHex)

	// Parse básico (en futuro codec8)
	parsed, err := codec.ParseCodec8E(data)
	if err != nil {
		fmt.Println("error parsing data:", err)
		return
	}

	// Imprimir resultado simulado (JSON provisional)
	fmt.Printf("🧩 Parsed data: %+v\n", parsed)
}
