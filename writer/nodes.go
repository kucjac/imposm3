package writer

import (
	"sync"

	"github.com/kucjac/imposm3/cache"
	"github.com/kucjac/imposm3/database"
	"github.com/kucjac/imposm3/element"
	"github.com/kucjac/imposm3/expire"
	geomp "github.com/kucjac/imposm3/geom"
	"github.com/kucjac/imposm3/geom/geos"
	"github.com/kucjac/imposm3/log"
	"github.com/kucjac/imposm3/mapping"
	"github.com/kucjac/imposm3/stats"
)

type NodeWriter struct {
	OsmElemWriter
	nodes        chan *element.Node
	pointMatcher mapping.NodeMatcher
}

func NewNodeWriter(
	osmCache *cache.OSMCache,
	nodes chan *element.Node,
	inserter database.Inserter,
	progress *stats.Statistics,
	matcher mapping.NodeMatcher,
	srid int,
) *OsmElemWriter {
	nw := NodeWriter{
		OsmElemWriter: OsmElemWriter{
			osmCache: osmCache,
			progress: progress,
			wg:       &sync.WaitGroup{},
			inserter: inserter,
			srid:     srid,
		},
		pointMatcher: matcher,
		nodes:        nodes,
	}
	nw.OsmElemWriter.writer = &nw
	return &nw.OsmElemWriter
}

func (nw *NodeWriter) loop() {
	geos := geos.NewGeos()
	geos.SetHandleSrid(nw.srid)
	defer geos.Finish()

	for n := range nw.nodes {
		nw.progress.AddNodes(1)
		if matches := nw.pointMatcher.MatchNode(n); len(matches) > 0 {
			nw.NodeToSrid(n)
			point, err := geomp.Point(geos, *n)
			if err != nil {
				if errl, ok := err.(ErrorLevel); !ok || errl.Level() > 0 {
					log.Println("[warn]: ", err)
				}
				continue
			}

			geom, err := geomp.AsGeomElement(geos, point)
			if err != nil {
				log.Println("[warn]: ", err)
				continue
			}

			inserted := false
			if nw.limiter != nil {
				parts, err := nw.limiter.Clip(geom.Geom)
				if err != nil {
					log.Println("[warn]: ", err)
					continue
				}
				if len(parts) >= 1 {
					if err := nw.inserter.InsertPoint(n.OSMElem, geom, matches); err != nil {
						log.Println("[warn]: ", err)
						continue
					}
					inserted = true
				}
			} else {
				if err := nw.inserter.InsertPoint(n.OSMElem, geom, matches); err != nil {
					log.Println("[warn]: ", err)
					continue
				}
				inserted = true
			}

			if inserted && nw.expireor != nil {
				expire.ExpireProjectedNode(nw.expireor, *n, nw.srid)
			}
		}
	}
	nw.wg.Done()
}
