package pbf

import (
	"bytes"
	"compress/zlib"
	"io"
	"os"
	"testing"

	"github.com/gogo/protobuf/proto"

	"github.com/kucjac/imposm3/element"
	"github.com/kucjac/imposm3/parser/pbf/internal/osmpbf"
)

func BenchmarkHello(b *testing.B) {
	b.StopTimer()
	pbf, err := open("./monaco-20150428.osm.pbf")
	if err != nil {
		b.Fatal(err)
	}

	for pos := range pbf.BlockPositions() {
		b.StartTimer()
		for i := 0; i < b.N; i++ {
			readPrimitiveBlock(pos)
		}
		return
		// for {
		// 	stringtable := NewStringTable(block.GetStringtable())

		// 	for _, group := range block.Primitivegroup {
		// 		dense := group.GetDense()
		// 		ReadDenseNodes(dense, block, stringtable)
		// 	}
		// }
		// return
	}

}

func BenchmarkParser(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p, err := NewParser("./monaco-20150428.osm.pbf")
		if err != nil {
			b.Fatal(err)
		}

		coords := make(chan []element.Node)
		nodes := make(chan []element.Node)
		ways := make(chan []element.Way)
		relations := make(chan []element.Relation)
		go func() {
			for range coords {
			}
		}()
		go func() {
			for range nodes {
			}
		}()
		go func() {
			for range ways {
			}
		}()
		go func() {
			for range relations {
			}
		}()

		p.Parse(coords, nodes, ways, relations)

		close(coords)
		close(nodes)
		close(ways)
		close(relations)
	}
}

func BenchmarkPrimitiveBlock(b *testing.B) {
	b.StopTimer()

	file, err := os.Open("./monaco-20150428.osm.pbf")
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	var block = &osmpbf.PrimitiveBlock{}
	var blob = &osmpbf.Blob{}

	var size = 79566
	var offset int64 = 155

	blobData := make([]byte, size)
	file.Seek(offset, 0)
	io.ReadFull(file, blobData)
	err = proto.Unmarshal(blobData, blob)
	if err != nil {
		b.Fatal("unmarshaling error blob: ", err)
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		buf := bytes.NewBuffer(blob.GetZlibData())
		r, err := zlib.NewReader(buf)
		if err != nil {
			b.Fatal("zlib error: ", err)
		}
		raw := make([]byte, blob.GetRawSize())
		io.ReadFull(r, raw)
		err = proto.Unmarshal(raw, block)
		if err != nil {
			b.Fatal("unmarshaling error: ", err)
		}
	}
}
