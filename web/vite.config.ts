import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The Go server embeds dist/ (web/embed.go) and serves dist/assets at
// /assets/. Development runs through `rootcause serve` on a tagged build.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    assetsDir: "assets",
  },
});
