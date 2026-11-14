import { useEffect, useRef } from "react";

// 动效只在登录和创作页运行，页面不可见或用户减少动态效果时停止逐帧绘制。
export default function ParticleField({ className = "" }) {
  const canvasRef = useRef(null);
  useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas.getContext("2d");
    if (!context) return;
    const motion = window.matchMedia("(prefers-reduced-motion: reduce)");
    let width = 0, height = 0, particles = [], frame = 0, previous = 0;
    function resize() {
      const rect = canvas.getBoundingClientRect();
      width = rect.width; height = rect.height;
      const ratio = Math.min(window.devicePixelRatio || 1, 2);
      canvas.width = width * ratio; canvas.height = height * ratio;
      context.setTransform(ratio, 0, 0, ratio, 0, 0);
      particles = Array.from({ length: Math.min(56, Math.max(18, Math.floor(width / 13))) }, () => ({
        x: Math.random() * width, y: Math.random() * height,
        vx: (Math.random() - .5) * .22, vy: -.12 - Math.random() * .18, radius: .8 + Math.random() * 1.5
      }));
      draw(false);
    }
    function draw(animate) {
      context.clearRect(0, 0, width, height);
      for (let i = 0; i < particles.length; i++) {
        const p = particles[i];
        if (animate) { p.x = (p.x + p.vx + width) % width; p.y = (p.y + p.vy + height) % height; }
        context.beginPath(); context.arc(p.x, p.y, p.radius, 0, Math.PI * 2);
        context.fillStyle = "rgba(164,145,242,.65)"; context.fill();
        for (let j = i + 1; j < particles.length; j++) {
          const q = particles[j], distance = Math.hypot(p.x - q.x, p.y - q.y);
          if (distance > 105) continue;
          context.beginPath(); context.moveTo(p.x, p.y); context.lineTo(q.x, q.y);
          context.strokeStyle = `rgba(170,148,250,${(1 - distance / 105) * .16})`; context.lineWidth = .6; context.stroke();
        }
      }
    }
    function tick(now) {
      if (now - previous > 32) { draw(true); previous = now; }
      frame = requestAnimationFrame(tick);
    }
    function sync() {
      cancelAnimationFrame(frame);
      if (!document.hidden && !motion.matches) frame = requestAnimationFrame(tick);
      else draw(false);
    }
    const observer = new ResizeObserver(resize);
    observer.observe(canvas);
    document.addEventListener("visibilitychange", sync);
    motion.addEventListener("change", sync);
    resize(); sync();
    return () => { cancelAnimationFrame(frame); observer.disconnect(); document.removeEventListener("visibilitychange", sync); motion.removeEventListener("change", sync); };
  }, []);
  return <canvas className={`particle-field ${className}`} ref={canvasRef} aria-hidden="true"/>;
}
