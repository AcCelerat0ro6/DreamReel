import { useEffect, useRef, useState } from "react";
import { Brand } from "./AppShell";
import ParticleField from "./ParticleField";
import Icon from "./Icon";
import "./LoginPage.css";

export default function LoginPage({ session, onNavigate, request }) {
  const [mode, setMode] = useState("login");
  const [form, setForm] = useState({ account: "", password: "", nickname: "" });
  const [message, setMessage] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [visiblePassword, setVisiblePassword] = useState(false);
  const abortRef = useRef(null);
  const busyRef = useRef(false);
  const registering = mode === "register";
  useEffect(() => () => abortRef.current?.abort(), []);

  function switchMode(next) { if (!submitting) { setMode(next); setMessage(""); } }
  async function handleSubmit(event) {
    event.preventDefault();
    if (busyRef.current) return;
    busyRef.current = true;
    const controller = new AbortController();
    abortRef.current = controller;
    setSubmitting(true); setMessage("");
    try {
      if (registering) await request("/api/users", { method: "POST", signal: controller.signal, body: { account: form.account.trim(), password: form.password, nickname: form.nickname.trim() } });
      const result = await request("/api/sessions", { method: "POST", signal: controller.signal, body: { account: form.account.trim(), password: form.password } });
      const profile = await request("/api/users/me", { token: result.access_token, signal: controller.signal });
      if (controller.signal.aborted) return;
      session.setAuth(result.access_token, profile);
      onNavigate("/recommend");
    } catch (error) {
      if (!controller.signal.aborted) setMessage(error.message || "暂时无法登录，请稍后重试");
    } finally {
      busyRef.current = false;
      if (!controller.signal.aborted) setSubmitting(false);
    }
  }

  return <main className="auth-screen">
    <section className="auth-scene" aria-label="DreamReel 灵感空间">
      <ParticleField/>
      <div className="auth-scene-brand"><Brand onClick={() => onNavigate("/timeline")}/></div>
      <div className="auth-orbits" aria-hidden="true">
        <span className="auth-orbit auth-orbit-one"/><span className="auth-orbit auth-orbit-two"/>
        <span className="auth-film auth-film-back"/><span className="auth-film auth-film-front"><Icon name="play_arrow" size={44}/></span>
        <i className="auth-spark auth-spark-one"/><i className="auth-spark auth-spark-two"/>
      </div>
      <div className="auth-scene-copy"><span>IMAGINATION IN MOTION</span><h1>下一帧，<br/><em>遇见你的想象。</em></h1><p>看见灵感，也让灵感被看见。</p></div>
      <button className="auth-explore" onClick={() => onNavigate("/timeline")}>先去发现灵感<Icon name="arrow_outward" size={17}/></button>
    </section>
    <section className="auth-entry">
      <button className="auth-mobile-back" onClick={() => onNavigate("/timeline")}><Icon name="arrow_back" size={17}/>发现灵感</button>
      <div className="auth-entry-card">
        <div className="auth-entry-heading"><span className="auth-entry-mark"><Icon name="auto_awesome" size={23}/></span><p>欢迎来到 DreamReel</p><h2>{registering ? "开启你的创作旅程" : "很高兴，再次遇见你"}</h2></div>
        <div className={`auth-tabs ${registering ? "is-register" : ""}`} role="group" aria-label="账户方式">
          <span aria-hidden="true"/><button type="button" disabled={submitting} aria-pressed={!registering} onClick={() => switchMode("login")}>登录</button><button type="button" disabled={submitting} aria-pressed={registering} onClick={() => switchMode("register")}>注册</button>
        </div>
        <form key={mode} className={`auth-entry-form ${registering ? "slide-forward" : "slide-back"}`} onSubmit={handleSubmit} aria-busy={submitting}>
          <label><span>账号</span><input name="account" autoComplete="username" value={form.account} required disabled={submitting} placeholder="请输入账号" onChange={event => setForm({ ...form, account: event.target.value })}/></label>
          {registering && <label><span>昵称</span><input name="nickname" autoComplete="nickname" value={form.nickname} required disabled={submitting} placeholder="希望大家如何称呼你" onChange={event => setForm({ ...form, nickname: event.target.value })}/></label>}
          <label><span>密码</span><span className="auth-password-field"><input name="password" autoComplete={registering ? "new-password" : "current-password"} type={visiblePassword ? "text" : "password"} value={form.password} required disabled={submitting} placeholder={registering ? "设置你的密码" : "请输入密码"} onChange={event => setForm({ ...form, password: event.target.value })}/><button type="button" aria-label={visiblePassword ? "隐藏密码" : "显示密码"} aria-pressed={visiblePassword} onClick={() => setVisiblePassword(value => !value)}><Icon name={visiblePassword ? "lock" : "visibility"} size={17}/></button></span></label>
          {message && <p className="auth-entry-error" role="alert">{message}</p>}
          <button className="auth-submit" disabled={submitting}>{submitting ? "正在连接你的灵感世界…" : registering ? "注册并开启旅程" : "登录，继续探索"}{!submitting && <Icon name="arrow_forward" size={18}/>}</button>
        </form>
        <p className="auth-switch-note">{registering ? "已经有账号？" : "还没有账号？"}<button type="button" disabled={submitting} onClick={() => switchMode(registering ? "login" : "register")}>{registering ? "立即登录" : "加入 DreamReel"}</button></p>
      </div>
      <span className="auth-entry-footer">每一帧想象，都有回响。</span>
    </section>
  </main>;
}
