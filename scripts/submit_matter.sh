#!/bin/sh
set -eu

: "${IMAGE_PATH:?set IMAGE_PATH to a local image}"

curl --fail-with-body --request POST http://localhost:8080/submissions \
  --form 'matter_id=matter-1042' \
  --form 'stage=signed_document_delivery' \
  --form 'caption=Signed engagement letter for client delivery' \
  --form "image=@${IMAGE_PATH}"
