import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";

export default defineConfig({
  plugins: [react()],
  resolve: {
    // tsconfig の paths と合わせる
    alias: { "@": resolve(import.meta.dirname, ".") },
  },
  test: {
    environment: "jsdom",
    globals: false,
    // 生成物はテストしない。直す先は openapi.yaml
    exclude: ["node_modules/**", ".next/**", "**/*.gen.ts"],
  },
});
