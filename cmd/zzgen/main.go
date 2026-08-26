package main

import (
	"fmt"
	"strings"

	"lnreadertui/internal/epub"
)

func main() {
	var paras []string
	for i := 0; i < 300; i++ {
		paras = append(paras, "これは長い本文の段落です。ページ移動のテストのための文章が続きます。かえるのつづき。")
	}
	b := epub.Bundle{
		ID: "long-test", Title: "长文测试书", Author: "测试",
		Volumes:  []epub.VolumeRef{{Title: "v1", Count: 1}},
		Chapters: []epub.ChapterRef{{Title: "第一章 長い章", HTML: "<p>" + strings.Join(paras, "</p><p>") + "</p>"}},
	}
	if err := epub.Write("/tmp/long-book.epub", b); err != nil {
		panic(err)
	}
	fmt.Println("ok /tmp/long-book.epub")
}
