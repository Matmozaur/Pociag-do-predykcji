FROM node:22.23.4-alpine3.24 AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --ignore-scripts

FROM node:22.23.4-alpine3.24 AS prod-deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --ignore-scripts --omit=dev

FROM node:22.23.4-alpine3.24 AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN API_URL=http://api:8080 npm run build

FROM node:22.23.4-alpine3.24 AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=builder /app/package.json ./package.json
COPY --from=prod-deps /app/node_modules ./node_modules
# next start writes its cache under .next, so the runtime user must own it.
COPY --chown=node:node --from=builder /app/.next ./.next
COPY public ./public
USER node
EXPOSE 3000
CMD ["npm", "run", "start"]
