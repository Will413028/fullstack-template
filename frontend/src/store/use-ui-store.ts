import { create } from "zustand";

type UiState = {
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
  toggleSidebar: () => void;
};

export const useUiStore = create<UiState>((set) => ({
  sidebarOpen: false,
  setSidebarOpen: (open) => set({ sidebarOpen: open }),
  toggleSidebar: () => set((state) => ({ sidebarOpen: !state.sidebarOpen })),
}));
