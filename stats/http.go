package stats

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/kucjac/imposm3/log"
)

func StartHttpPProf(bind string) {
	go func() {
		log.Println(http.ListenAndServe(bind, nil))
	}()
}
