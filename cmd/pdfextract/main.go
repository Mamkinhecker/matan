package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/ledongthuc/pdf"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("usage: pdfextract input.pdf output.txt")
	}

	file, reader, err := pdf.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	output, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer output.Close()

	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		pageText, err := reader.Page(pageNumber).GetPlainText(nil)
		if err != nil {
			log.Printf("page %d: %v", pageNumber, err)
			continue
		}
		fmt.Fprintf(output, "\n\n===== PAGE %d =====\n\n%s", pageNumber, pageText)
	}

	log.Printf("extracted %d pages to %s", reader.NumPage(), filepath.Clean(os.Args[2]))
}
