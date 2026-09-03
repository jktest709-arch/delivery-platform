FROM node:22-alpine AS builder

WORKDIR /src
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci

COPY frontend/ .
ARG VITE_API_BASE_URL=/api
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
RUN npm run build

FROM nginx:1.27-alpine

ENV BACKEND_UPSTREAM=backend:8080
COPY deploy/nginx/default.conf.template /etc/nginx/templates/default.conf.template
COPY --from=builder /src/dist /usr/share/nginx/html

EXPOSE 80
