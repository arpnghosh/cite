package main

import (
	"bytes"
	"errors"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

var (
	ErrUnmarshalFrontMatter = errors.New("error while unmarshaling front matter")
	ErrConvertMarkdown      = errors.New("error while converting markdown to HTML")
)

type FrontMatter struct {
	Title       string    `yaml:"title"`
	Date        time.Time `yaml:"date"`
	Description string    `yaml:"description"`
	Draft       bool      `yaml:"draft"`
	Layout      string    `yaml:"layout"`
}

type Page struct {
	Title       string
	Description string
	Date        string
	RawDate     time.Time
	Content     template.HTML
	Slug        string
	OutputPath  string
	SourcePath  string
}

type SiteConfig struct {
	SiteTitle       string
	SiteName        string
	SiteAuthor      string
	SiteDescription string
	Year            int
}

type PageData struct {
	Page
	SiteConfig
}

type IndexData struct {
	SiteConfig
	Posts []Page
}

func main() {
	var pages []Page
	// var buf bytes.Buffer
	err := filepath.Walk("content", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".md" {
			return nil
		}

		content, _ := os.ReadFile(path)

		var fm FrontMatter

		parts := bytes.SplitN(content, []byte("---"), 3)

		if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
			return ErrUnmarshalFrontMatter
		}

		if fm.Draft {
			return nil
		}

		// convert the part[2] to html using goldmark
		// then construct the page and append that to []Page array
		var buf bytes.Buffer
		md := goldmark.New(
			goldmark.WithExtensions(extension.Footnote),
			goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		)
		if err := md.Convert(parts[2], &buf); err != nil {
			return ErrConvertMarkdown
		}

		filename := filepath.Base(path)
		slug := strings.TrimSuffix(filename, ".md")
		slug = strings.ToLower(slug)
		slug = strings.ReplaceAll(slug, " ", "-")

		page := Page{
			Title:       fm.Title,
			Description: fm.Description,
			Date:        fm.Date.Format("2006-01-02"),
			RawDate:     fm.Date,
			Content:     template.HTML(buf.String()),
			Slug:        slug,
			SourcePath:  path,
			OutputPath:  filepath.Join("build", slug+".html"),
		}

		pages = append(pages, page)
		return nil
	})
	if err != nil {
		// fmt.Errorf("Error while walking the content folder")
	}

	// Sort pages by date (newest first)
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].RawDate.After(pages[j].RawDate)
	})

	if err := os.MkdirAll("build", 0o755); err != nil {
		panic(err)
	}
	if err := os.MkdirAll("build/css", 0o755); err != nil {
		panic(err)
	}

	cssFiles := []string{"style.css"}
	for _, cssFile := range cssFiles {
		src, err := os.ReadFile(filepath.Join("templates", cssFile))
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join("build/css", cssFile), src, 0o644); err != nil {
			panic(err)
		}
	}

	tmpl, err := template.ParseGlob("templates/*.html")
	if err != nil {
		panic(err)
	}

	siteConfig := SiteConfig{
		SiteTitle:       "Arpan Ghosh",
		SiteName:        "Arpan Ghosh",
		SiteAuthor:      "Arpan Ghosh",
		SiteDescription: "blog",
		Year:            time.Now().Year(),
	}

	for _, page := range pages {
		data := PageData{
			Page:       page,
			SiteConfig: siteConfig,
		}

		var output bytes.Buffer
		if err := tmpl.ExecuteTemplate(&output, "base.html", data); err != nil {
			panic(err)
		}

		if err := os.WriteFile(page.OutputPath, output.Bytes(), 0o644); err != nil {
			panic(err)
		}
	}

	// Generate index.html
	indexData := IndexData{
		SiteConfig: siteConfig,
		Posts:      pages,
	}
	var indexOutput bytes.Buffer
	if err := tmpl.ExecuteTemplate(&indexOutput, "index.tmpl.html", indexData); err != nil {
		panic(err)
	}
	if err := os.WriteFile("build/index.html", indexOutput.Bytes(), 0o644); err != nil {
		panic(err)
	}
}
