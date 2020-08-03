package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"
)

func main() {
	file, err := os.OpenFile("../access.log",os.O_APPEND|os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		panic(fmt.Sprintf("open file err %s", err))
	}
	defer file.Close()

	for {
		for i := 1; i < 4; i++ {
			now := time.Now()
			rand.Seed(now.UnixNano())
			paths := []string{"/foo", "/bar", "/baz", "/qux", "/foo", "/bar", "/bar", "/bar"}
			path := paths[rand.Intn(len(paths))]
			requestTime := rand.Float64()
			if path == "/foo" {
				requestTime = requestTime + 1.4
			}

			scheme := "http"
			if now.UnixNano()/1000%2 == 1 {
				scheme = "https"
			}
			dateTime := now.Format(time.RFC3339)
			code := 200
			if now.Unix()%10 == 1 {
				code = 500
			}
			bytesSend := rand.Intn(1000) + 500
			if path == "/foo" {
				bytesSend = bytesSend + 1000
			}
			line := fmt.Sprintf("172.0.0.12 - - [%s] %s \"GET %s HTTP/1.0\" %d %d \"-\" \"KeepAliveClient\" \"-\" - %.3f\n", dateTime, scheme, path, code, bytesSend, requestTime)
			_, err := file.Write([]byte(line))
			if err != nil {
				log.Println("write to file err: ", err)
			}
		}
		log.Println("local log generate success.")
		time.Sleep(time.Millisecond * 200)
	}
}
