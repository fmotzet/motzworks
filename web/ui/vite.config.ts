import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Builds into the Go-embedded directory. During dev, /api is proxied to the
// running `motzworks serve` instance on :8080.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../../internal/web/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
