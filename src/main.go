package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	highlighting "github.com/yuin/goldmark-highlighting/v2"
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

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Footnote,
			highlighting.NewHighlighting(
				highlighting.WithStyle("gruvbox"),
			),
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)

	err := filepath.Walk("content", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing %s: %w", path, err)
		}
		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".md" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		var fm FrontMatter

		parts := bytes.SplitN(content, []byte("---"), 3)
		if len(parts) < 3 {
			return fmt.Errorf("invalid frontmatter in %s", path)
		}

		if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
			return fmt.Errorf("%s %w", path, ErrUnmarshalFrontMatter)
		}

		if fm.Draft {
			return nil
		}

		// convert the part[2] to html using goldmark
		// then construct the page and append that to []Page array
		var buf bytes.Buffer
		if err := md.Convert(parts[2], &buf); err != nil {
			return fmt.Errorf("failed to convert %s: %w", path, ErrConvertMarkdown)
		}

		filename := filepath.Base(path)
		slug := strings.TrimSuffix(filename, ".md")
		slug = strings.ToLower(slug)
		slug = strings.ReplaceAll(slug, " ", "-")

		sanitized := bluemonday.UGCPolicy().Sanitize(buf.String())

		page := Page{
			Title:       fm.Title,
			Description: fm.Description,
			Date:        fm.Date.Format("2006-01-02"),
			RawDate:     fm.Date,
			Content:     template.HTML(sanitized),
			Slug:        slug,
			SourcePath:  path,
			OutputPath:  filepath.Join("build", slug+".html"),
		}

		pages = append(pages, page)
		return nil
	})
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Sort pages by date (newest first)
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].RawDate.After(pages[j].RawDate)
	})

	if err := os.MkdirAll("build", 0o755); err != nil {
		log.Fatalf("Error: %v", err)
	}
	if err := os.MkdirAll("build/css", 0o755); err != nil {
		log.Fatalf("Error: %v", err)
	}

	cssFiles := []string{"style.css"}
	for _, cssFile := range cssFiles {
		src, err := os.ReadFile(filepath.Join("public/style", cssFile))
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
		if err := os.WriteFile(filepath.Join("build/css", cssFile), src, 0o644); err != nil {
			log.Fatalf("Error: %v", err)
		}
	}

	// Copy font files
	fontDirs := []string{"Inter", "Crimson", "IosevkaMono", "utopia"}
	for _, fontDir := range fontDirs {
		srcDir := filepath.Join("public/fonts", fontDir)
		dstDir := filepath.Join("build/fonts", fontDir)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			log.Fatalf("Error: %v", err)
		}
		err := filepath.Walk(srcDir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("%w in %s", err, path)
			}
			if info.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			if ext == ".ttf" || ext == ".woff2" || ext == ".otf" {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				dstPath := filepath.Join(dstDir, filepath.Base(path))
				if err := os.WriteFile(dstPath, data, 0o644); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	tmpl, err := template.New("").Funcs(template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("Jan 02 2006")
		},
	}).ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal(err)
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
			log.Fatalf("Error: %v", err)
		}

		if err := os.WriteFile(page.OutputPath, output.Bytes(), 0o644); err != nil {
			log.Fatalf("Error: %v", err)
		}
	}

	// Generate index.html
	indexData := IndexData{
		SiteConfig: siteConfig,
		Posts:      pages,
	}
	var indexOutput bytes.Buffer
	if err := tmpl.ExecuteTemplate(&indexOutput, "index.tmpl.html", indexData); err != nil {
		log.Fatalf("Error: %v", err)
	}
	if err := os.WriteFile("build/index.html", indexOutput.Bytes(), 0o644); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
