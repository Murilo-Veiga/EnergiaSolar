FROM node:22-alpine AS build

WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci

COPY . .
# URL da api-go embutida no bundle estático em tempo de build — o browser
# do usuário chama essa URL diretamente, não passa pelo container nginx.
ARG VITE_API_URL=http://localhost:8090
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
