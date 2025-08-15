package config

import (
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

func TuneGC() {
	mode := strings.ToLower(os.Getenv("GC_MODE"))

	switch mode {
	case "off":
		debug.SetGCPercent(-1)
		log.Println("[GC] Disabled (SetGCPercent(-1))")
	case "high":
		debug.SetGCPercent(1000)
		log.Println("[GC] High threshold mode (SetGCPercent(1000))")
	case "custom":
		// Example: GC_MODE=custom,GC_PERCENT=750
		p := os.Getenv("GC_PERCENT")
		if p != "" {
			if val, err := strconv.Atoi(p); err == nil {
				debug.SetGCPercent(val)
				log.Printf("[GC] Custom mode (SetGCPercent(%d))\n", val)
				return
			}
		}
		debug.SetGCPercent(100)
		log.Println("[GC] Invalid GC_PERCENT, fallback to default 100")
	default:
		debug.SetGCPercent(100)
		log.Println("[GC] Normal mode (SetGCPercent(100))")
	}
}
