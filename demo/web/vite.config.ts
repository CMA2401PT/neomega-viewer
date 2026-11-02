import { defineConfig } from "vite";
export default defineConfig({
  resolve: { dedupe: ["three"] },
  build: {
    rolldownOptions: {
      input: { blocks: "index.html", subchunk: "subchunk.html" },
    },
  },
  server: {
    fs: { allow: ["../../.."] },
    proxy: {
      "/api": "http://127.0.0.1:2410",
      "/assets/entity": "http://127.0.0.1:2410",
      "/assets/atlas.png": "http://127.0.0.1:2410",
      "/assets/THIRD_PARTY_NOTICES.txt": "http://127.0.0.1:2410",
    },
  },
});
