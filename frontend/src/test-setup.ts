import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// Vitest runs without globals, so Testing Library cannot unmount on its own;
// leftover renders would keep duplicate element ids in the document.
afterEach(cleanup);
