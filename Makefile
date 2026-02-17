.PHONY: build layer deploy destroy clean fmt validate

BINARY := bootstrap
ZIP := bootstrap.zip
LAYER_DIR := layers/tesseract
LAYER_ZIP := $(LAYER_DIR)/tesseract-layer.zip
TF_DIR := terraform

build:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/lambda/
	zip $(ZIP) $(BINARY)

layer:
	cd $(LAYER_DIR) && bash build.sh

deploy: build
	cd $(TF_DIR) && terraform init && terraform apply

destroy:
	cd $(TF_DIR) && terraform destroy

clean:
	rm -f $(BINARY) $(ZIP) $(LAYER_ZIP)

fmt:
	cd $(TF_DIR) && terraform fmt

validate:
	cd $(TF_DIR) && terraform validate
