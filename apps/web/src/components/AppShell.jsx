import { useEffect, useRef, useState } from "react";
import Icon from "./Icon";
import "./AccountMenu.css";

const navigation = [
  { route: "/timeline", label: "发现灵感", icon: "explore" },
  { route: "/recommend", label: "为您推荐", icon: "auto_awesome" },
  { route: "/following", label: "我的关注", icon: "subscriptions" },
  { route: "/hotfeed", label: "热门作品", icon: "local_fire_department" }
];

export function Brand({ onClick, showTagline = false }) {
  return <button className="wordmark" onClick={onClick} aria-label="DreamReel 首页">
    <span className="logo-symbol"><Icon name="play_arrow" size={23}/><i/></span>
    <span>Dream<span className="wordmark-accent">Reel</span>{showTagline && <small>让想象，自由放映</small>}</span>
  </button>;
}

export default function AppShell({ children, user, route, authenticated, unreadCount, onNavigate, onLogout, onEditProfile, searchValue = "", onSearch }) {
  const feedPage = navigation.some(item => item.route === route);
  const ownProfile = route === "/profile";
  const creationPage = route === "/create" || route === "/upload";
  const searchRef = useRef(null);
  const accountRef = useRef(null);
  const triggerRef = useRef(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);

  useEffect(() => { setMenuOpen(false); }, [route, authenticated]);
  useEffect(() => {
    function handleKey(event) {
      if (event.key === "/" && !event.ctrlKey && !event.metaKey && !event.target?.closest?.("input, textarea, select, [contenteditable]")) {
        event.preventDefault(); setSearchOpen(true); searchRef.current?.focus();
      }
      if (event.key === "Escape") {
        setSearchOpen(false); searchRef.current?.blur();
        if (menuOpen) { setMenuOpen(false); triggerRef.current?.focus(); }
      }
    }
    function dismiss(event) { if (!accountRef.current?.contains(event.target)) setMenuOpen(false); }
    window.addEventListener("keydown", handleKey);
    document.addEventListener("pointerdown", dismiss);
    return () => { window.removeEventListener("keydown", handleKey); document.removeEventListener("pointerdown", dismiss); };
  }, [menuOpen]);
  useEffect(() => {
    if (menuOpen) accountRef.current?.querySelector('[role="menuitem"]')?.focus();
  }, [menuOpen]);

  const destination = path => onNavigate(authenticated ? path : "/auth");
  function openAccount() {
    if (!authenticated) return onNavigate("/auth");
    if (!ownProfile) return onNavigate("/profile");
    setMenuOpen(value => !value);
  }
  function moveMenuFocus(event) {
    const buttons = [...event.currentTarget.querySelectorAll('[role="menuitem"]')];
    const index = buttons.indexOf(document.activeElement);
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const next = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : (index + (event.key === "ArrowDown" ? 1 : -1) + buttons.length) % buttons.length;
    buttons[next]?.focus();
  }
  const navButton = (path, label, icon) => <button key={path} className={`sidebar-link ${route === path ? "active" : ""}`} aria-current={route === path ? "page" : undefined} onClick={() => onNavigate(path)}><Icon name={icon}/><span>{label}</span></button>;

  return <div className={`app-shell ${feedPage ? "is-feed" : ""}`}>
    <header className="top-nav">
      <Brand onClick={() => onNavigate("/timeline")} showTagline={creationPage}/>
      <label className={`search-box ${searchOpen ? "search-open" : ""}`} onClick={() => { setSearchOpen(true); searchRef.current?.focus(); }}>
        <Icon name="search" size={19}/><input ref={searchRef} aria-label="搜索灵感" placeholder="寻找下一帧灵感" value={searchValue} onChange={event => onSearch?.(event.target.value)}/>
        {searchValue ? <button aria-label="清除搜索" onClick={() => onSearch?.("")}><Icon name="close" size={15}/></button> : <kbd>/</kbd>}
      </label>
      <div className="top-actions">
        <button className="upload-button" aria-label="开始创作" onClick={() => destination("/create")}><Icon name="add" size={18}/><span>开始创作</span></button>
        <button className="icon-button badge-button" aria-label="通知" onClick={() => destination("/messages")}><Icon name="notifications"/>{unreadCount > 0 && <span className="notification-dot"/>}</button>
        <div className="account-menu-wrap" ref={accountRef} onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setMenuOpen(false); }}>
          <button id="account-trigger" ref={triggerRef} className={`avatar-button ${authenticated ? "" : "guest"}`}
            aria-label={!authenticated ? "登录" : ownProfile ? "个人资料菜单" : "前往个人主页"}
            aria-haspopup={authenticated && ownProfile ? "menu" : undefined}
            aria-expanded={authenticated && ownProfile ? menuOpen : undefined}
            aria-controls={menuOpen ? "account-menu" : undefined}
            onClick={openAccount} onKeyDown={event => { if (authenticated && ownProfile && event.key === "ArrowDown") { event.preventDefault(); setMenuOpen(true); } }}>
            {authenticated ? <img src={user?.avatar_url || "/avatar.svg"} alt="我的头像"/> : "登录 / 注册"}
          </button>
          {authenticated && ownProfile && menuOpen && <div id="account-menu" className="account-menu" role="menu" aria-label="个人资料操作" onKeyDown={moveMenuFocus}>
            <button role="menuitem" onClick={() => { setMenuOpen(false); onEditProfile(); }}><Icon name="manage_accounts" size={17}/>编辑个人资料</button>
            <button role="menuitem" className="account-signout" onClick={() => { setMenuOpen(false); onLogout(); }}><Icon name="logout" size={17}/>退出登录</button>
          </div>}
        </div>
      </div>
    </header>
    <div className="app-body">
      <aside className="sidebar">
        {!feedPage && <p className="nav-section-label">探索社区</p>}
        <nav aria-label="探索社区">{navigation.map(item => navButton(item.route, item.label, item.icon))}{navButton("/models", "模型榜单", "leaderboard")}</nav>
        <div className="sidebar-divider"/>
        <p className="nav-section-label">我的空间</p>
        <nav aria-label="我的空间">
          <button className={`sidebar-link ${ownProfile ? "active" : ""}`} onClick={() => destination("/profile")}><Icon name="person"/><span>创作者主页</span></button>
          <button className={`sidebar-link ${creationPage ? "active" : ""}`} onClick={() => destination("/create")}><Icon name="movie"/><span>创作</span></button>
          <button className={`sidebar-link ${route === "/messages" ? "active" : ""}`} onClick={() => destination("/messages")}><Icon name="chat_bubble"/><span>互动消息</span>{unreadCount > 0 && <span className="nav-badge">{unreadCount > 99 ? "99+" : unreadCount}</span>}</button>
        </nav>
      </aside>
      <div className="page-outlet" key={route}>{children}</div>
    </div>
  </div>;
}
