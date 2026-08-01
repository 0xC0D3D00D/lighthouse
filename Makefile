.PHONY: build web test run clean mocks

mocks:
	rm -rf generated/mocks && mockery

web:
	cd web && npm install && npm run build

build: web
	go build -o lighthouse ./cmd/lighthouse

test:
	go test ./...

run: build
	./lighthouse

clean:
	rm -rf lighthouse web/dist web/.output web/.nuxt
