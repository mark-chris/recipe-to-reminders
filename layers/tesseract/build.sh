#!/usr/bin/env bash
set -euo pipefail

# Build Tesseract 5.x Lambda layer for Amazon Linux 2023 ARM64.
# Requires Docker with QEMU binfmt support for ARM64 emulation.
#
# Output: tesseract-layer.zip (~15-20MB) in this directory.
#
# Usage: cd layers/tesseract && bash build.sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_ZIP="${SCRIPT_DIR}/tesseract-layer.zip"
CONTAINER_NAME="tesseract-layer-builder"
LEPTONICA_VERSION="1.84.1"
TESSERACT_VERSION="5.5.0"
TESSDATA_URL="https://github.com/tesseract-ocr/tessdata_best/raw/main/eng.traineddata"

echo "==> Building Tesseract ${TESSERACT_VERSION} Lambda layer for ARM64..."
echo "    Leptonica: ${LEPTONICA_VERSION}"
echo "    This may take 10-15 minutes on x86 hosts (QEMU emulation)."

# Clean up any previous builder container
docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true

docker run --name "${CONTAINER_NAME}" --platform linux/arm64 \
  amazonlinux:2023 bash -c "
set -euo pipefail

# Install build dependencies
dnf groupinstall -y 'Development Tools'
dnf install -y cmake libtiff-devel libjpeg-turbo-devel libpng-devel \
  zlib-devel wget tar gzip

# Build Leptonica
cd /tmp
wget -q https://github.com/DanBloomberg/leptonica/releases/download/${LEPTONICA_VERSION}/leptonica-${LEPTONICA_VERSION}.tar.gz
tar xzf leptonica-${LEPTONICA_VERSION}.tar.gz
cd leptonica-${LEPTONICA_VERSION}
mkdir build && cd build
cmake .. -DCMAKE_INSTALL_PREFIX=/opt -DBUILD_SHARED_LIBS=OFF -DCMAKE_POSITION_INDEPENDENT_CODE=ON
make -j\$(nproc)
make install

# Build Tesseract
cd /tmp
wget -q https://github.com/tesseract-ocr/tesseract/archive/refs/tags/${TESSERACT_VERSION}.tar.gz
tar xzf ${TESSERACT_VERSION}.tar.gz
cd tesseract-${TESSERACT_VERSION}
mkdir build && cd build
cmake .. -DCMAKE_INSTALL_PREFIX=/opt -DBUILD_SHARED_LIBS=OFF \
  -DCMAKE_POSITION_INDEPENDENT_CODE=ON \
  -DLeptonica_DIR=/opt/lib64/cmake/leptonica \
  -DBUILD_TRAINING_TOOLS=OFF -DDISABLE_LEGACY_ENGINE=ON
make -j\$(nproc)
make install

# Download trained data
mkdir -p /opt/share/tessdata
wget -q -O /opt/share/tessdata/eng.traineddata ${TESSDATA_URL}

# Verify
/opt/bin/tesseract --version
echo '==> Tesseract built successfully'
"

# Copy artifacts out
echo "==> Extracting layer artifacts..."
STAGING=$(mktemp -d)
trap 'rm -rf "${STAGING}"; docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true' EXIT
docker cp "${CONTAINER_NAME}:/opt/bin/tesseract" "${STAGING}/tesseract"
mkdir -p "${STAGING}/share/tessdata"
docker cp "${CONTAINER_NAME}:/opt/share/tessdata/eng.traineddata" "${STAGING}/share/tessdata/eng.traineddata"

# Also copy required shared libraries
mkdir -p "${STAGING}/lib"
docker cp "${CONTAINER_NAME}:/opt/lib64/." "${STAGING}/lib/" 2>/dev/null || true
docker cp "${CONTAINER_NAME}:/opt/lib/." "${STAGING}/lib/" 2>/dev/null || true

# Create layer zip with Lambda layer structure
# Lambda layers extract to /opt, so paths should be relative:
#   bin/tesseract -> /opt/bin/tesseract
#   share/tessdata/eng.traineddata -> /opt/share/tessdata/eng.traineddata
#   lib/ -> /opt/lib/ (shared libs if any)
echo "==> Creating layer zip..."
rm -f "${OUTPUT_ZIP}"
cd "${STAGING}"
mkdir -p bin
mv tesseract bin/
zip -r "${OUTPUT_ZIP}" bin/ share/ lib/

SIZE=$(du -h "${OUTPUT_ZIP}" | cut -f1)
echo "==> Layer zip created: ${OUTPUT_ZIP} (${SIZE})"
# Cleanup handled by EXIT trap
