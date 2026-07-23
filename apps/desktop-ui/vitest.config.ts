import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// 별도 파일로 분리: vite.config.ts는 tauri dev(고정 포트, HMR)에 특화돼 있어
// 테스트 설정과 섞지 않는다.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    globals: true,
  },
});
