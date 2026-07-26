.PHONY: backend-test frontend-dev frontend-build frontend-test frontend-e2e test docker-up docker-down

backend-test:
	cd backend && go test ./...

frontend-dev:
	npm install --prefix frontend
	npm run dev --prefix frontend

frontend-build:
	npm install --prefix frontend
	npm run build --prefix frontend

frontend-test:
	npm install --prefix frontend
	npm run test --prefix frontend

frontend-e2e:
	npm install --prefix frontend
	npm run test:e2e --prefix frontend

test: backend-test frontend-build frontend-test

docker-up:
	docker compose up --build

docker-down:
	docker compose down
