"use client";

export default function AnimatedBackground() {
  return (
    <div className="fixed inset-0 overflow-hidden pointer-events-none">
      <div className="absolute inset-0 bg-gradient-to-br from-[#0a0a1a] via-[#0f0f2e] to-[#1a0a2e]" />

      <div
        className="absolute top-[-10%] left-[-5%] w-[500px] h-[500px] rounded-full opacity-30 blur-[100px] animate-orb-move-1"
        style={{ background: "radial-gradient(circle, #06b6d4, transparent 70%)" }}
      />
      <div
        className="absolute top-[40%] right-[-10%] w-[600px] h-[600px] rounded-full opacity-25 blur-[100px] animate-orb-move-2"
        style={{ background: "radial-gradient(circle, #8b5cf6, transparent 70%)" }}
      />
      <div
        className="absolute bottom-[-10%] left-[20%] w-[450px] h-[450px] rounded-full opacity-20 blur-[100px] animate-orb-move-3"
        style={{ background: "radial-gradient(circle, #ec4899, transparent 70%)" }}
      />
      <div
        className="absolute top-[20%] left-[50%] w-[300px] h-[300px] rounded-full opacity-15 blur-[80px] animate-orb-move-1"
        style={{ background: "radial-gradient(circle, #3b82f6, transparent 70%)" }}
      />

      <div className="absolute inset-0 opacity-[0.03]" style={{
        backgroundImage: `radial-gradient(circle at 1px 1px, rgba(255,255,255,0.3) 1px, transparent 0)`,
        backgroundSize: "40px 40px",
      }} />
    </div>
  );
}
