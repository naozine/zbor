# Transcribe - 音声文字起こしツール

Sherpa-ONNX + ReazonSpeechを使用した日本語音声文字起こしコマンドラインツール

## 前提条件

- Go 1.25以上
- CGO対応のCコンパイラ（gcc/clang）
- macOSの場合: Xcode Command Line Tools

```bash
# macOSの場合
xcode-select --install
```

## セットアップ

### 1. モデルのダウンロード

```bash
# プロジェクトルートから実行
cd /Users/hnao/GolandProjects/zbor

# ReazonSpeechモデルをダウンロード
curl -SL -O https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01.tar.bz2

# モデルを展開
tar xvf sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01.tar.bz2 -C models/

# ダウンロードしたアーカイブを削除（任意）
rm sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01.tar.bz2
```

### 2. ビルド

```bash
# transcribeコマンドをビルド
go build -o zbor-transcribe ./cmd/transcribe

# または、CGOを明示的に有効化してビルド
CGO_ENABLED=1 go build -o zbor-transcribe ./cmd/transcribe
```

## 使い方

### 基本的な使用例

```bash
# 音声ファイルを文字起こし（標準出力）
./zbor-transcribe -i audio.wav

# 結果をファイルに保存
./zbor-transcribe -i audio.wav -o transcript.txt

# 詳細ログ付きで実行
./zbor-transcribe -i audio.wav -v
```

### 出力形式

#### テキスト形式（デフォルト）
```bash
./zbor-transcribe -i audio.wav -format text
```

#### JSON形式（タイムスタンプ付き）
```bash
./zbor-transcribe -i audio.wav -format json -o result.json
```

出力例:
```json
{
  "text": "これはテストの音声です。日本語の音声認識を試しています。",
  "duration": 1.23
}
```

#### SRT字幕形式
```bash
./zbor-transcribe -i audio.wav -format srt -o subtitles.srt
```

### オプション

```
  -i string
        Input audio file (WAV format)
  -o string
        Output file (default: stdout)
  -format string
        Output format: text, json, srt (default "text")
  -model string
        Model directory path (default "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01")
  -threads int
        Number of threads for inference (default 2)
  -v    Verbose output
```

## 対応音声形式

- **WAV形式**のみ対応
- 推奨サンプリングレート: **16000 Hz**
- チャンネル: モノラル推奨

### 音声形式の変換（ffmpegを使用）

他の形式から変換する場合:

```bash
# MP3からWAVへ変換
ffmpeg -i input.mp3 -ar 16000 -ac 1 output.wav

# MP4の音声トラックを抽出してWAVへ変換
ffmpeg -i video.mp4 -vn -ar 16000 -ac 1 audio.wav
```

## トラブルシューティング

### モデルが見つからないエラー

```
Error: encoder file not found
```

モデルをダウンロードしてください:
```bash
curl -SL -O https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01.tar.bz2
tar xvf sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01.tar.bz2 -C models/
```

### CGOエラー

```
cgo: C compiler not found
```

C/C++コンパイラをインストールしてください:
- **macOS**: `xcode-select --install`
- **Linux**: `apt-get install build-essential` (Ubuntu/Debian)
- **Windows**: MinGW-w64またはMSYS2

### 共有ライブラリエラー

macOSで以下のエラーが出る場合:
```
dyld: Library not loaded
```

CGOを有効化してビルドしてください:
```bash
CGO_ENABLED=1 go build -o zbor-transcribe ./cmd/transcribe
```

## パフォーマンス

- **モデルサイズ**: 約300MB（int8量子化版使用）
- **推論速度**: リアルタイムの約2〜3倍速（CPU依存）
- **メモリ使用量**: 約500MB〜1GB
- **GPU不要**: CPUのみで動作

## 技術スタック

- **Sherpa-ONNX**: ONNX Runtimeベースの音声認識フレームワーク
- **ReazonSpeech**: 35,000時間の日本語データで訓練された高精度ASRモデル
- **Zipformer**: Transformer派生の最新アーキテクチャ

## ライセンス

- このツール: MIT License
- ReazonSpeechモデル: Apache 2.0 License
