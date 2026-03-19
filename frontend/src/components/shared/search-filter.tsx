"use client";

import { Search } from "lucide-react";
import { useQueryState } from "nuqs";
import { useEffect, useState } from "react";
import { useDebounce } from "@/hooks/use-debounce";

interface SearchFilterProps {
  placeholder?: string;
  className?: string;
}

export function SearchFilter({
  placeholder = "Search...",
  className,
}: SearchFilterProps) {
  const [query, setQuery] = useQueryState("q", { defaultValue: "" });
  const [inputValue, setInputValue] = useState(query);
  const debouncedValue = useDebounce(inputValue);

  useEffect(() => {
    setQuery(debouncedValue || null);
  }, [debouncedValue, setQuery]);

  return (
    <div className={`relative ${className}`}>
      <Search
        size={16}
        className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500"
      />
      <input
        type="text"
        value={inputValue}
        onChange={(e) => setInputValue(e.target.value)}
        placeholder={placeholder}
        className="w-full pl-10 pr-4 py-2.5 bg-zinc-900 border border-zinc-800 rounded-lg text-white placeholder:text-zinc-500 focus:outline-none focus:border-zinc-600 transition-colors"
      />
    </div>
  );
}
