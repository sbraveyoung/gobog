package main

import (
	"sync"

	_ "github.com/SmartBrave/gobog/src/server"
)

func main() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	wg.Wait()
}
