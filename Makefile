.PHONY: install dev-backend dev-frontend test test-backend test-frontend build clean

install:
	cd frontend && npm ci
	cd backend && go mod download

dev-backend:
	cd backend && go run ./cmd/fakemex -config ../config/local.env

dev-frontend:
	cd frontend && npm start

test: test-backend test-frontend

test-backend:
	cd backend && go test ./...
	cd backend && go vet ./...

test-frontend:
	cd frontend && npm test
	cd frontend && npm run test:e2e

build:
	cd frontend && npm run build
	mkdir -p backend/bin
	cd backend && go build -o bin/fakemex ./cmd/fakemex

clean:
	cd frontend && npm run ng -- cache clean

