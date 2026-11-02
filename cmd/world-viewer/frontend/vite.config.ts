import { defineConfig } from "vite";

export default defineConfig({
  resolve: {
    dedupe: ["three"],
  },
  build: { outDir: "../../../viewassets/bundle/web", emptyOutDir: true },
  server: {
    fs: { allow: ["../../../../.."] },
    proxy: {
      "/api": "http://127.0.0.1:2412",
      "/resources": "http://127.0.0.1:2412",
    },
  },
});
