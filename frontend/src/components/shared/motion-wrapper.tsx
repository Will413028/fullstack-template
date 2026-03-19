"use client";

import type { Variants } from "framer-motion";
import { motion, useInView } from "framer-motion";
import { useRef } from "react";
import { slideUp } from "@/lib/animations";

interface MotionWrapperProps {
  children: React.ReactNode;
  variants?: Variants;
  className?: string;
  once?: boolean;
  amount?: number;
}

export function MotionWrapper({
  children,
  variants = slideUp,
  className,
  once = true,
  amount = 0.3,
}: MotionWrapperProps) {
  const ref = useRef(null);
  const isInView = useInView(ref, { once, amount });

  return (
    <motion.div
      ref={ref}
      variants={variants}
      initial="hidden"
      animate={isInView ? "visible" : "hidden"}
      className={className}
    >
      {children}
    </motion.div>
  );
}
