package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsteiman/tickets-general/backend/internal/tokens"
)

func main() {
	id := uuid.MustParse(os.Args[1])
	dsn := os.Getenv("DATABASE_URL")
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	var nonce []byte
	if err := pool.QueryRow(context.Background(), `SELECT qr_nonce FROM tickets WHERE id = $1`, id).Scan(&nonce); err != nil {
		panic(err)
	}
	rawKey := os.Getenv("TOKEN_SIGNING_KEY")
	keyBytes, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil || len(keyBytes) < 16 {
		keyBytes = []byte(rawKey)
	}
	tok, err := tokens.Sign(tokens.PurposeView, id, nonce, keyBytes)
	if err != nil {
		panic(err)
	}
	fmt.Print(tok)
}
