package main

import (
	"github.com/beaujr/kong-log-worldmap/server"
)

func main() {
	las := server.NewLogApiServer()
	las.StartServer()

}
