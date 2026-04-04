package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaYAML 对应 schemas/xxx/schema.yaml 的结构
type SchemaYAML struct {
	ID            string    `yaml:"id" json:"id"`
	Name          string    `yaml:"name" json:"name"`
	Version       string    `yaml:"version" json:"version"`
	Author        string    `yaml:"author" json:"author"`
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
		if err := validateSchema(meta); err != nil {
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

func validateSchema(meta *SchemaYAML) error {
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
	if len(meta.Variants) == 0 {
		return fmt.Errorf("缺少 variants")
	}
	return nil
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
