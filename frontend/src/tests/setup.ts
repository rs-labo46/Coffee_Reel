import "@testing-library/jest-dom/vitest";

import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(() => {
  cleanup();
  document.cookie = "csrf_token=; Max-Age=0; Path=/";
  document.cookie = "refresh_token=; Max-Age=0; Path=/";
});
