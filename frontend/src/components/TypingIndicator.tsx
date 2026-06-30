"use client";

import { motion } from "framer-motion";

export default function TypingIndicator() {
  return (
    <motion.div
      initial={{ opacity: 0, x: -30, y: 10 }}
      animate={{ opacity: 1, x: 0, y: 0 }}
      exit={{ opacity: 0, x: -30, y: 10 }}
      transition={{ type: "spring", stiffness: 300, damping: 30 }}
      className="flex items-end mb-3"
    >
      <div className="w-8 h-8 rounded-full bg-gradient-to-br from-accent-cyan/30 to-accent-violet/30 border border-white/10 mr-3 bg-cover bg-center bg-no-repeat flex-shrink-0" />
      <div className="glass rounded-2xl rounded-bl-sm px-4 py-3 border border-white/10">
        <div className="flex items-center h-[17px] gap-1.5">
          {[0, 1, 2].map((i) => (
            <motion.div
              key={i}
              className="w-2 h-2 rounded-full"
              style={{
                background: `linear-gradient(135deg, ${
                  i === 0 ? "#06b6d4" : i === 1 ? "#8b5cf6" : "#ec4899"
                }, ${
                  i === 0 ? "#8b5cf6" : i === 1 ? "#ec4899" : "#06b6d4"
                })`,
              }}
              animate={{
                y: [0, -8, 0],
                scale: [1, 1.3, 1],
                opacity: [0.4, 1, 0.4],
              }}
              transition={{
                duration: 1.4,
                repeat: Infinity,
                delay: i * 0.15,
                ease: "easeInOut",
              }}
            />
          ))}
        </div>
      </div>
    </motion.div>
  );
}
