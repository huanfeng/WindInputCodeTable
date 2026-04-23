# WindInput 第三方词库

风输入法的第三方输入方案仓库。支持从输入法设置中自动下载并导入。

## 可用方案

| 方案 | 类型 | 说明 |
|------|------|------|
| 晶晶码 (jjm) | 形码 | 晶晶码输入方案，支持纯码表和拼音混合两种模式 |
| 创码 (chuangma) | 形码 | 创码输入方案，支持纯码表和拼音混合两种模式 |
| 飞鹰声形码 (fysxm) | 形码 | 飞鹰声形码输入方案，支持纯码表和拼音混合两种模式 |
| 虎字 (tiger) | 形码 | 虎字输入方案，支持纯码表和拼音混合两种模式 |
| 虎词 (tigress) | 形码 | 虎词输入方案，支持纯码表和拼音混合两种模式 |
| 新世纪五笔06 (wubi06) | 五笔 | 新世纪五笔06输入方案，支持纯码表和拼音混合两种模式 |
| 五笔98 (wubi98) | 五笔 | 五笔98输入方案，支持纯码表和拼音混合两种模式 |
| 朕码 (zm) | 形码 | 朕码输入方案，支持纯码表和拼音混合两种模式 |

## 客户端对接

客户端通过 GitHub Release 获取词库：

1. 获取最新 Release 的 `index.json`
2. 展示方案列表供用户选择
3. 下载对应的 zip 包，校验 SHA256
4. 解压到 `schemas/` 目录，注册方案

**API 示例：**

```
# 获取最新 release 信息
GET https://api.github.com/repos/huanfeng/WindInputCodeTable/releases/latest

# 下载 index.json（从 release assets 中获取 URL）
GET {index.json 的 browser_download_url}

# 下载方案压缩包
GET {zip 的 browser_download_url}
```

## 目录结构

```
schemas/
└── {方案ID}/
    ├── schema.yaml           # 方案元信息（作者维护，不会打包）
    ├── {id}.schema.yaml      # 方案配置文件
    └── {id}/
        └── {id}.dict.yaml    # 词典文件（Rime 格式）
```

目录结构与打包后的 zip 内部结构完全一致（排除 `schema.yaml`）。

## 贡献指南

欢迎提交新的输入方案！请按以下步骤操作：

### 1. 创建方案目录

在 `schemas/` 下创建以方案 ID 命名的目录（英文小写 + 数字，如 `wubi86`）。

### 2. 编写 `schema.yaml`

```yaml
id: your_schema_id
name: "方案名称"
version: "1.0.0"
author: "你的名字"
source: "来源说明"
description: "方案描述"
category: "xingma"           # xingma | wubi | shuangpin | pinyin | other
icon_label: "图"
min_app_version: "1.0.0"

variants:
  - id: your_schema_id
    name: "变体名称"
    schema_file: "your_schema_id.schema.yaml"
    default: true
```

### 3. 添加方案文件

- `.schema.yaml` — 方案引擎配置
- `dicts/*.dict.yaml` — Rime 格式词典文件

### 4. 提交 PR

Push 到 `main` 分支后，GitHub Actions 会自动构建并发布 Release。

## 构建

手动构建：

```bash
cd scripts
go run build.go
```

产物输出到 `dist/` 目录。
