.PHONY: frontend-dev frontend-build docker-up docker-down

frontend-dev:
	npm install --prefix frontend
	npm run dev --prefix frontend

frontend-build:
	npm install --prefix frontend
	npm run build --prefix frontend

docker-up:
	docker compose up --build

docker-down:
	docker compose down
