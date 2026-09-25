#!/usr/bin/env sh
set -eu

MODEL_DIR=${COGNIGO_MODEL_DIR:-models/mobilenetv2}
MODEL_PATH="$MODEL_DIR/mobilenetv2-7.onnx"
LABELS_PATH=${COGNIGO_LABELS_PATH:-imagenet_classes.txt}
ORT_VERSION=${COGNIGO_ONNX_RUNTIME_VERSION:-1.22.0}
ORT_DIR=${COGNIGO_ONNX_RUNTIME_DIR:-.local/onnxruntime}

case "$(uname -s):$(uname -m)" in
  Linux:x86_64)
    ORT_PLATFORM=linux-x64
    ORT_LIBRARY="$ORT_DIR/lib/libonnxruntime.so.$ORT_VERSION"
    ;;
  Linux:aarch64|Linux:arm64)
    ORT_PLATFORM=linux-aarch64
    ORT_LIBRARY="$ORT_DIR/lib/libonnxruntime.so.$ORT_VERSION"
    ;;
  Darwin:x86_64)
    ORT_PLATFORM=osx-x86_64
    ORT_LIBRARY="$ORT_DIR/lib/libonnxruntime.$ORT_VERSION.dylib"
    ;;
  Darwin:arm64)
    ORT_PLATFORM=osx-arm64
    ORT_LIBRARY="$ORT_DIR/lib/libonnxruntime.$ORT_VERSION.dylib"
    ;;
  *)
    printf 'Unsupported platform: %s %s\n' "$(uname -s)" "$(uname -m)" >&2
    exit 1
    ;;
esac

mkdir -p "$MODEL_DIR"

if [ ! -s "$ORT_LIBRARY" ]; then
  TEMP_DIR=$(mktemp -d)
  trap 'find "$TEMP_DIR" -depth -delete' EXIT HUP INT TERM
  ARCHIVE="$TEMP_DIR/onnxruntime.tgz"
  curl --fail --location --retry 3 --output "$ARCHIVE" \
    "https://github.com/microsoft/onnxruntime/releases/download/v$ORT_VERSION/onnxruntime-$ORT_PLATFORM-$ORT_VERSION.tgz"
  mkdir -p "$ORT_DIR"
  tar -xzf "$ARCHIVE" --strip-components=1 -C "$ORT_DIR"
fi

if [ ! -s "$MODEL_PATH" ]; then
  curl --fail --location --retry 3 --output "$MODEL_PATH" \
    "https://github.com/onnx/models/raw/main/validated/vision/classification/mobilenet/model/mobilenetv2-7.onnx"
fi

if [ ! -s "$LABELS_PATH" ]; then
  curl --fail --location --retry 3 --output "$LABELS_PATH" \
    "https://raw.githubusercontent.com/onnx/models/main/validated/vision/classification/synset.txt"
fi

printf 'ONNX Runtime: %s\nModel: %s\nLabels: %s\n' "$ORT_LIBRARY" "$MODEL_PATH" "$LABELS_PATH"
