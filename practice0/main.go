package main

import (
	"fmt"
	"time"
)

func main() {
	b := make([]byte, 100)

	go func() {
		for {
			filler(b[:50], '0', '1') // Заполняем первую половину
			time.Sleep(time.Second)
		}
	}()

	go func() {
		for {
			filler(b[50:], 'X', 'Y')
			time.Sleep(time.Second)
		}
	}()
	go func() {
		for {
			fmt.Println(string(b))
			time.Sleep(time.Second)
		}
	}()
	// зацикливается навсегда
	select {}
}

func filler(b []byte, ifzero byte, ifnot byte) {
	for i := range b {
		if i%2 == 0 {
			b[i] = ifzero
		} else {
			b[i] = ifnot
		}
	}
}
