package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

func main() {
	http.HandleFunc("/events", sseHandler)

	if err := http.ListenAndServe(":1122", nil); err != nil {
		log.Fatalf("unable to start server: %s", err.Error())
	}
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	setCommonHeaders(w)

	memT := time.NewTicker(time.Second)
	defer memT.Stop()

	cpuT := time.NewTicker(time.Second)
	defer cpuT.Stop()

	clientGone := r.Context().Done()

	rc := http.NewResponseController(w)

	for {
		select {
		case <-clientGone:
			fmt.Println("client has disconnected")
			return
		case <-memT.C:
			m, err := mem.VirtualMemory()
			if err != nil {
				log.Printf("unable to get mem: %s", err)
				return
			}

			_, err = fmt.Fprintf(w, "event:mem\ndata:Total: %.2f MB, Used: %.2f MB, Perc: %.2f%%\n\n",
				float64(m.Total)/1024/1024, float64(m.Used)/1024/1024, m.UsedPercent)
			if err != nil {
				log.Printf("unable to write: %s", err)
				return
			}

			if err := rc.Flush(); err != nil {
				log.Printf("unable to flush: %s", err)
				return
			}
		case <-cpuT.C:
			c, err := cpu.Times(false)
			if err != nil {
				log.Printf("unable to get cpu: %s", err)
				return
			}

			_, err = fmt.Fprintf(w, "event:cpu\ndata:User: %.2f, Sys: %.2f, Idle: %.2f\n\n",
				c[0].User, c[0].System, c[0].Idle)
			if err != nil {
				log.Printf("unable to write: %s", err)
				return
			}

			if err := rc.Flush(); err != nil {
				log.Printf("unable to flush: %s", err)
				return
			}
		}
	}
}

func setCommonHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	w.Header().Set("Access-Control-Allow-Origin", "*")
}
