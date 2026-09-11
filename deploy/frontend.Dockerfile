FROM node:20-alpine AS deps

WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

FROM node:20-alpine AS builder

WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY frontend ./
ENV NEXT_TELEMETRY_DISABLED=1
# next.config.mjs sets output:"standalone", so this writes a self-contained
# server under .next/standalone alongside only the node_modules it traced.
RUN npm run build

FROM node:20-alpine AS runner

WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup -g 10001 -S nextjs && adduser -u 10001 -S nextjs -G nextjs

# Three copies, not the whole tree. standalone brings server.js and the traced
# dependencies; static and public are the assets it serves but does not bundle.
# The image used to carry every production dependency whether the app reached
# for it or not.
COPY --from=builder --chown=nextjs:nextjs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nextjs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nextjs /app/public ./public

USER nextjs

# Documentation, not configuration: server.js listens on PORT, which Render
# sets, and falls back to 3000 only when nothing says otherwise.
EXPOSE 3000

# HOSTNAME has to be 0.0.0.0 or the standalone server binds to localhost inside
# the container and nothing outside it can connect.
ENV HOSTNAME=0.0.0.0
ENV PORT=3000

CMD ["node", "server.js"]
