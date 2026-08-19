# jellycli

日本語 | [English](README.md)

[![Build](https://github.com/minittupoyo/jellycli/actions/workflows/build.yml/badge.svg)](https://github.com/minittupoyo/jellycli/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/minittupoyo/jellycli)](https://github.com/minittupoyo/jellycli/releases/latest)

`jellycli` は、Jellyfinサーバー上のメディアをターミナルで閲覧・再生するGo製CLI/TUIクライアントです。ターミナルUIには [Bubble Tea](https://github.com/charmbracelet/bubbletea)、再生には `mpv` を使用します。

## 主な機能

- ライブラリ、シリーズ、シーズン、エピソード、映画などを階層型TUIで閲覧
- 「視聴を続ける」「次のエピソード」「最近追加されたメディア」を表示
- エピソードだけでなく、シリーズや映画などの作品も検索
- Direct Play、Direct Stream、Transcodeから再生方法を選択
- JSON IPCによる`mpv`制御とJellyfinへの再生状況同期
- 続きからの再生と、開始・進捗・停止・視聴済み状態の通知
- XDG互換の非公開ファイルに設定と認証情報を保存
- スクリプトから利用できる非対話型CLIコマンド

## 対応環境

| 環境 | ビルド | 再生対応 |
| --- | --- | --- |
| Linux amd64/arm64 | 配布中 | 対応 |
| macOS amd64/arm64 | 配布中 | 実験的対応 |
| Windows amd64/arm64 | 配布中 | 未対応（名前付きパイプIPCの実装が必要） |

## インストール

[最新リリース](https://github.com/minittupoyo/jellycli/releases/latest)から、お使いの環境向けのアーカイブをダウンロードしてください。

Linux amd64の場合:

```sh
version=0.1.0
curl -LO "https://github.com/minittupoyo/jellycli/releases/download/v${version}/jellycli-linux-amd64.tar.gz"
curl -LO "https://github.com/minittupoyo/jellycli/releases/download/v${version}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
tar -xzf jellycli-linux-amd64.tar.gz
install -m 0755 jellycli ~/.local/bin/jellycli
```

ソースからビルドする場合はGo 1.25以降をインストールして、次を実行します。

```sh
git clone https://github.com/minittupoyo/jellycli.git
cd jellycli
go build ./cmd/jellycli
```

再生には別途`mpv`をインストールし、`PATH`から実行できるようにしてください。

## クイックスタート

対話形式でログインします。

```sh
jellycli login
```

引数なしで実行すると、デフォルトでTUIが起動します。

```sh
jellycli
```

主な非対話型コマンド:

```sh
jellycli libraries
jellycli search "作品名"
jellycli play ITEM_ID
jellycli logout
```

すべてのコマンドは`jellycli help`、個別の使い方は`jellycli help COMMAND`で確認できます。

## TUIの操作

| キー | 操作 |
| --- | --- |
| `↑`/`k`、`↓`/`j` | 選択項目を移動 |
| `enter` | 選択項目を開く、または再生 |
| `esc` | 前の画面へ戻る、または検索をキャンセル |
| `/` | 検索 |
| `q`/`ctrl+c` | 終了 |

## 設定とセキュリティ

設定はXDG Base Directoryの規約に従って保存されます。認証情報を含むファイルは所有者だけが読み書きできる権限で作成されます。サーバーURL、アクセストークン、ユーザー名、デバイスIDを含むファイルやログをコミット、公開、貼り付けしないでください。

認証情報が漏洩した可能性がある場合は、Jellyfin側でアクセストークンを無効化してから、`jellycli login`を再実行してください。

## ドキュメント

- [アーキテクチャ](docs/architecture.ja.md)
- [詳細設計と実装履歴](docs/design.md)
- [トラブルシューティング](docs/troubleshooting.ja.md)
- [コントリビューションガイド](CONTRIBUTING.ja.md)
- [変更履歴](CHANGELOG.md)

## リリース

`v*`形式のバージョンタグをpushすると、GitHub Actionsが対応環境向けアーカイブとチェックサムを生成し、GitHub Releaseを公開します。

## ライセンス

このリポジトリには、現時点でソフトウェアライセンスが付与されていません。リポジトリが公開されていること自体は、コードの複製、変更、再配布を許可するものではありません。今後のリリースでライセンスを追加する可能性があります。
