// +build generate

package main

import (
	"log"

	"github.com/asdawn/mbtileserver/handlers"
	"github.com/shurcooL/vfsgen"
)

func main() {
	err := vfsgen.Generate(handlers.Assets, vfsgen.Options{
		PackageName:  "handlers",
		BuildTags:    "!dev",
		VariableName: "Assets",
	})
	if err != nil {
		log.Fatalln(err)
	}
}
