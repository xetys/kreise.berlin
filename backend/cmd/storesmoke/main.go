package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/dsteiman/tickets-general/backend/internal/objectstore"
)

func main() {
	ctx := context.Background()
	c, err := objectstore.New(ctx, objectstore.Config{
		Endpoint:     os.Getenv("STORAGE_ENDPOINT"),
		Region:       os.Getenv("STORAGE_REGION"),
		Bucket:       os.Getenv("STORAGE_BUCKET"),
		AccessKey:    os.Getenv("STORAGE_ACCESS_KEY"),
		SecretKey:    os.Getenv("STORAGE_SECRET_KEY"),
		UsePathStyle: true,
	})
	if err != nil {
		panic(err)
	}

	key := "smoke/hello.txt"
	if err := c.Put(ctx, key, bytes.NewReader([]byte("hello world")), "text/plain"); err != nil {
		panic(err)
	}
	fmt.Println("PUT OK")

	body, ct, err := c.Get(ctx, key)
	if err != nil {
		panic(err)
	}
	defer body.Close()
	b, _ := io.ReadAll(body)
	fmt.Printf("GET OK: %q (content-type=%s)\n", string(b), ct)

	exists, err := c.Exists(ctx, key)
	if err != nil {
		panic(err)
	}
	fmt.Printf("EXISTS: %v\n", exists)

	if err := c.Delete(ctx, key); err != nil {
		panic(err)
	}
	fmt.Println("DELETE OK")

	exists, _ = c.Exists(ctx, key)
	fmt.Printf("EXISTS after delete: %v\n", exists)
}
