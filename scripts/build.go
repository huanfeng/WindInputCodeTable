package main

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// SchemaYAML 对应 schemas/xxx/schema.yaml 的结构
type SchemaYAML struct {
	ID            string    `yaml:"id" json:"id"`
	Name          string    `yaml:"name" json:"name"`
	Version       string    `yaml:"version" json:"version"`
	Author        string    `yaml:"author" json:"author"`
	Source        string    `yaml:"source" json:"source"`
	Description   string    `yaml:"description" json:"description"`
	Category      string    `yaml:"category" json:"category"`
	IconLabel     string    `yaml:"icon_label" json:"icon_label"`
	MinAppVersion string    `yaml:"min_app_version" json:"min_app_version"`
	Variants      []Variant `yaml:"variants" json:"variants"`
}

type Variant struct {
	ID         string `yaml:"id" json:"id"`
	Name       string `yaml:"name" json:"name"`
	SchemaFile string `yaml:"schema_file" json:"-"`
	Default    bool   `yaml:"default,omitempty" json:"default,omitempty"`
}

// IndexJSON 是最终输出的清单
type IndexJSON struct {
	Version    int           `json:"version"`
	ReleasedAt string        `json:"released_at"`
	Schemas    []SchemaEntry `json:"schemas"`
}

type SchemaEntry struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Version       string    `json:"version"`
	Author        string    `json:"author"`
	Source        string    `json:"source"`
	Description   string    `json:"description"`
	Category      string    `json:"category"`
	IconLabel     string    `json:"icon_label"`
	MinAppVersion string    `json:"min_app_version"`
	Variants      []Variant `json:"variants"`
	Download      Download  `json:"download"`
}

type Download struct {
	File   string `json:"file"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func main() {
	// 项目根目录 = scripts 的上级目录
	scriptDir, err := os.Getwd()
	if err != nil {
		fatal("获取工作目录失败: %v", err)
	}

	rootDir := filepath.Dir(scriptDir)
	schemasDir := filepath.Join(rootDir, "schemas")
	distDir := filepath.Join(rootDir, "dist")

	// 清理并创建 dist 目录
	os.RemoveAll(distDir)
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		fatal("创建 dist 目录失败: %v", err)
	}

	// 遍历 schemas/ 下的每个方案目录
	entries, err := os.ReadDir(schemasDir)
	if err != nil {
		fatal("读取 schemas 目录失败: %v", err)
	}

	var schemaEntries []SchemaEntry

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		schemaDir := filepath.Join(schemasDir, entry.Name())
		metaFile := filepath.Join(schemaDir, "schema.yaml")

		// 读取并解析 schema.yaml
		meta, err := parseSchemaYAML(metaFile)
		if err != nil {
			fatal("解析 %s 失败: %v", metaFile, err)
		}

		// 校验必填字段
		if err := validateSchema(meta, schemaDir); err != nil {
			fatal("校验 %s 失败: %v", entry.Name(), err)
		}

		fmt.Printf("正在处理方案: %s (%s)\n", meta.Name, meta.ID)

		// 收集需要打包的文件
		files, err := collectFiles(schemaDir, meta)
		if err != nil {
			fatal("收集 %s 文件失败: %v", entry.Name(), err)
		}

		// 打包为 zip
		zipName := fmt.Sprintf("%s-%s.zip", meta.ID, meta.Version)
		zipPath := filepath.Join(distDir, zipName)
		if err := createZip(zipPath, schemaDir, files); err != nil {
			fatal("打包 %s 失败: %v", zipName, err)
		}

		// 计算 SHA256 和文件大小
		hash, size, err := fileSHA256(zipPath)
		if err != nil {
			fatal("计算 %s SHA256 失败: %v", zipName, err)
		}

		fmt.Printf("  -> %s (%.1f KB, sha256:%s)\n", zipName, float64(size)/1024, hash[:12]+"...")

		schemaEntries = append(schemaEntries, SchemaEntry{
			ID:            meta.ID,
			Name:          meta.Name,
			Version:       meta.Version,
			Author:        meta.Author,
			Source:        meta.Source,
			Description:   meta.Description,
			Category:      meta.Category,
			IconLabel:     meta.IconLabel,
			MinAppVersion: meta.MinAppVersion,
			Variants:      meta.Variants,
			Download: Download{
				File:   zipName,
				Size:   size,
				SHA256: hash,
			},
		})
	}

	if len(schemaEntries) == 0 {
		fatal("未找到任何方案")
	}

	// 生成 index.json
	index := IndexJSON{
		Version:    1,
		ReleasedAt: time.Now().UTC().Format(time.RFC3339),
		Schemas:    schemaEntries,
	}

	indexPath := filepath.Join(distDir, "index.json")
	indexData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		fatal("序列化 index.json 失败: %v", err)
	}
	if err := os.WriteFile(indexPath, indexData, 0o644); err != nil {
		fatal("写入 index.json 失败: %v", err)
	}

	fmt.Printf("\n构建完成! 共 %d 个方案，产物目录: dist/\n", len(schemaEntries))
}

func parseSchemaYAML(path string) (*SchemaYAML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta SchemaYAML
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func validateSchema(meta *SchemaYAML, schemaDir string) error {
	if meta.ID == "" {
		return fmt.Errorf("缺少 id")
	}
	if meta.Name == "" {
		return fmt.Errorf("缺少 name")
	}
	if meta.Version == "" {
		return fmt.Errorf("缺少 version")
	}
	if meta.Author == "" {
		return fmt.Errorf("缺少 author")
	}
	if meta.Source == "" {
		return fmt.Errorf("缺少 source")
	}
	if meta.Description == "" {
		return fmt.Errorf("缺少 description")
	}
	if meta.Category == "" {
		return fmt.Errorf("缺少 category")
	}
	if meta.IconLabel == "" {
		return fmt.Errorf("缺少 icon_label")
	}
	if meta.MinAppVersion == "" {
		return fmt.Errorf("缺少 min_app_version")
	}
	if len(meta.Variants) == 0 {
		return fmt.Errorf("缺少 variants")
	}

	if !isValidCategory(meta.Category) {
		return fmt.Errorf("category 非法: %s", meta.Category)
	}
	if len([]rune(meta.IconLabel)) != 1 {
		return fmt.Errorf("icon_label 必须为单个字符: %q", meta.IconLabel)
	}

	defaultCount := 0
	seenVariantIDs := make(map[string]bool)
	for i, variant := range meta.Variants {
		if variant.ID == "" {
			return fmt.Errorf("variants[%d] 缺少 id", i)
		}
		if variant.Name == "" {
			return fmt.Errorf("variants[%d] 缺少 name", i)
		}
		if variant.SchemaFile == "" {
			return fmt.Errorf("variants[%d] 缺少 schema_file", i)
		}
		if seenVariantIDs[variant.ID] {
			return fmt.Errorf("variants[%d] id 重复: %s", i, variant.ID)
		}
		seenVariantIDs[variant.ID] = true
		if variant.Default {
			defaultCount++
		}
		if err := validateVariantFile(schemaDir, variant); err != nil {
			return err
		}
	}
	if defaultCount != 1 {
		return fmt.Errorf("variants 必须且只能有一个 default=true，当前为 %d", defaultCount)
	}
	return nil
}

type variantFile struct {
	Schema struct {
		ID     string `toml:"id"`
		Name   string `toml:"name"`
		Author string `toml:"author"`
	} `toml:"schema"`
	Engine struct {
		Type  string `toml:"type"`
		Mixed struct {
			PrimarySchema   string `toml:"primary_schema"`
			SecondarySchema string `toml:"secondary_schema"`
		} `toml:"mixed"`
	} `toml:"engine"`
	Dictionaries []variantDict `toml:"dictionaries"`
}

type variantDict struct {
	Path string `toml:"path"`
	Type string `toml:"type"`
}

func validateVariantFile(schemaDir string, variant Variant) error {
	path := filepath.Join(schemaDir, variant.SchemaFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", variant.SchemaFile, err)
	}

	var vf variantFile
	if err := toml.Unmarshal(data, &vf); err != nil {
		return fmt.Errorf("解析 %s 失败: %w", variant.SchemaFile, err)
	}

	if vf.Schema.ID != variant.ID {
		return fmt.Errorf("%s 的 schema.id 必须与 variant.id 一致: %s != %s", variant.SchemaFile, vf.Schema.ID, variant.ID)
	}
	if vf.Schema.Name == "" {
		return fmt.Errorf("%s 缺少 schema.name", variant.SchemaFile)
	}
	if vf.Schema.Author == "" {
		return fmt.Errorf("%s 缺少 schema.author", variant.SchemaFile)
	}
	if !isValidEngineType(vf.Engine.Type) {
		return fmt.Errorf("%s 的 engine.type 非法: %s", variant.SchemaFile, vf.Engine.Type)
	}

	hasMixedRef := vf.Engine.Type == "mixed" && (vf.Engine.Mixed.PrimarySchema != "" || vf.Engine.Mixed.SecondarySchema != "")
	if len(vf.Dictionaries) == 0 && !hasMixedRef {
		return fmt.Errorf("%s 缺少 dictionaries", variant.SchemaFile)
	}

	for i, dict := range vf.Dictionaries {
		if dict.Path == "" {
			return fmt.Errorf("%s 的 dictionaries[%d].path 不能为空", variant.SchemaFile, i)
		}
		if !isValidDictType(dict.Type) {
			return fmt.Errorf("%s 的 dictionaries[%d].type 非法: %s", variant.SchemaFile, i, dict.Type)
		}

		dictPath := filepath.Join(schemaDir, filepath.FromSlash(dict.Path))
		if _, err := os.Stat(dictPath); err != nil {
			return fmt.Errorf("%s 引用的词典不存在: %s", variant.SchemaFile, dict.Path)
		}

		if dict.Type == "rime_codetable" || dict.Type == "rime_pinyin" {
			if err := validateRimeImports(dictPath); err != nil {
				return fmt.Errorf("%s 校验 import_tables 失败: %w", dict.Path, err)
			}
		}
	}

	return nil
}

func validateRimeImports(dictPath string) error {
	imports, err := parseRimeImportTables(dictPath)
	if err != nil {
		return err
	}

	dir := filepath.Dir(dictPath)
	for _, name := range imports {
		importPath := filepath.Join(dir, filepath.FromSlash(name+".dict.yaml"))
		if _, err := os.Stat(importPath); err != nil {
			return fmt.Errorf("缺少 import_tables 词典 %s", filepath.ToSlash(filepath.Join(filepath.Base(dir), name+".dict.yaml")))
		}
	}

	return nil
}

func parseRimeImportTables(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var headerLines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		headerLines = append(headerLines, line)
		if strings.TrimSpace(line) == "..." {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var header struct {
		ImportTables []string `yaml:"import_tables"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(headerLines, "\n")), &header); err != nil {
		return nil, err
	}

	return header.ImportTables, nil
}

func isValidCategory(category string) bool {
	switch category {
	case "xingma", "wubi", "shuangpin", "pinyin", "other":
		return true
	default:
		return false
	}
}

func isValidEngineType(engineType string) bool {
	switch engineType {
	case "codetable", "pinyin", "mixed":
		return true
	default:
		return false
	}
}

func isValidDictType(dictType string) bool {
	switch dictType {
	case "codetable", "rime_codetable", "rime_pinyin":
		return true
	default:
		return false
	}
}

// collectFiles 收集方案目录中需要打包的文件（排除 schema.yaml 元信息）
func collectFiles(schemaDir string, meta *SchemaYAML) ([]string, error) {
	var files []string
	err := filepath.Walk(schemaDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// 排除元信息文件，只打包实际的方案文件和词典
		relPath, _ := filepath.Rel(schemaDir, path)
		if relPath == "schema.yaml" {
			return nil
		}
		files = append(files, relPath)
		return nil
	})
	return files, err
}

// createZip 创建压缩包，源目录结构即为 zip 内部结构（排除 schema.yaml）
func createZip(zipPath, schemaDir string, files []string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for _, relPath := range files {
		srcPath := filepath.Join(schemaDir, relPath)
		archivePath := filepath.ToSlash(relPath)

		if err := addFileToZip(w, srcPath, archivePath); err != nil {
			return fmt.Errorf("添加 %s 失败: %w", relPath, err)
		}
	}
	return nil
}

func addFileToZip(w *zip.Writer, srcPath, archivePath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = archivePath
	header.Method = zip.Deflate

	dest, err := w.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(dest, src)
	return err
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), size, nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", args...)
	os.Exit(1)
}
