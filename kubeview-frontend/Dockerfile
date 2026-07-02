# NEXT_PUBLIC_* variables are inlined into the client bundle at build time,
# so the backend URL is a build ARG: one image per backend URL.
FROM node:22-alpine AS build
WORKDIR /src

COPY package.json package-lock.json ./
RUN npm ci

ARG NEXT_PUBLIC_API_BASE=http://localhost:5501/api
ENV NEXT_PUBLIC_API_BASE=$NEXT_PUBLIC_API_BASE

COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
# Bind all interfaces: in Kubernetes, HOSTNAME defaults to the pod name and
# the standalone server would skip 127.0.0.1, breaking kubectl port-forward.
ENV HOSTNAME=0.0.0.0

COPY --from=build --chown=node:node /src/.next/standalone ./
COPY --from=build --chown=node:node /src/.next/static ./.next/static
COPY --from=build --chown=node:node /src/public ./public

EXPOSE 3000
USER node
CMD ["node", "server.js"]
