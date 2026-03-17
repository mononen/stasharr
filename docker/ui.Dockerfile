FROM node:22-alpine AS dev
WORKDIR /app
COPY web/package*.json ./
RUN npm install -g pnpm@latest-10
RUN pnpm install
COPY web/ .
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]

FROM node:22-alpine AS builder
WORKDIR /app
COPY web/package*.json ./
RUN npm install -g pnpm@latest-10
RUN pnpm install
COPY web/ .
ARG VITE_API_URL
ENV VITE_API_URL=${VITE_API_URL}
RUN npm run build

FROM nginx:alpine AS production
COPY --from=builder /app/dist /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
