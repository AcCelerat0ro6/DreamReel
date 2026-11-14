// Local SVG icons keep the interface usable without an external icon font.
const paths = {
  visibility: <><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/></>,
  videocam_off: <><path d="m2 2 20 20M4 7H3v12h14v-3m-9-9h9v5l5-4v10l-5-4"/></>,
  check_circle: <><circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/></>,
  add: <path d="M12 5v14M5 12h14"/>,
  explore: <><circle cx="12" cy="12" r="9"/><path d="m16 8-2.5 5.5L8 16l2.5-5.5Z"/></>,
  arrow_upward: <path d="M12 20V4m-6 6 6-6 6 6"/>,
  arrow_downward: <path d="M12 4v16m-6-6 6 6 6-6"/>,
  home: <><path d="m3 10 9-7 9 7v10a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1Z" /></>,
  auto_awesome: <><path d="m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5L12 3Z" /><path d="m20 2 .5 1.5L22 4l-1.5.5L20 6l-.5-1.5L18 4l1.5-.5L20 2Z" /></>,
  subscriptions: <><rect x="3" y="8" width="18" height="13" rx="2" /><path d="M6 5h12M8 2h8" /><path d="m10 12 5 3-5 3Z" fill="currentColor" stroke="none" /></>,
  local_fire_department: <><path d="M12 3c1 4-3 5-3 8-1-1-1-3-1-3-3 3-4 5-4 7a8 8 0 0 0 16 0c0-4-4-9-8-12Z" /><path d="M12 13c-1 2-3 3-3 5a3 3 0 0 0 6 0c0-2-2-4-3-5Z" /></>,
  leaderboard: <><path d="M3 21h18M5 17v-6h4v6M10 17V4h4v13M15 17V8h4v9" /></>,
  notifications: <><path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4" /></>,
  search: <><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 5 5" /></>,
  upload: <><path d="M12 16V3m-5 5 5-5 5 5M4 15v5a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-5" /></>,
  publish: <><path d="M12 20V7m-5 5 5-5 5 5M4 3h16" /></>,
  person: <><circle cx="12" cy="8" r="4" /><path d="M4 21v-2a8 8 0 0 1 16 0v2" /></>,
  login: <><path d="M14 3h6v18h-6M3 12h12m-4-4 4 4-4 4" /></>,
  logout: <><path d="M10 3H4v18h6m0-9h11m-4-4 4 4-4 4" /></>,
  favorite: <><path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0l-1 1-1-1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z" /></>,
  chat_bubble: <><path d="M21 11.5a8.5 8.5 0 0 1-8.5 8.5H4l-2 2V11.5a9.5 9.5 0 0 1 19 0Z" /><path d="M7 9h9M7 13h6" /></>,
  bookmark: <><path d="M6 3h12v18l-6-4-6 4V3Z" /></>,
  share: <><circle cx="18" cy="5" r="3" /><circle cx="6" cy="12" r="3" /><circle cx="18" cy="19" r="3" /><path d="m8.6 10.5 6.8-4M8.6 13.5l6.8 4" /></>,
  hourglass_top: <><path d="M6 3h12v4l-6 5-6-5V3ZM6 21h12v-4l-6-5-6 5v4ZM5 3h14M5 21h14" /></>,
  lock: <><rect x="5" y="10" width="14" height="11" rx="2" /><path d="M8 10V7a4 4 0 0 1 8 0v3M12 14v3" /></>,
  sync_problem: <><path d="M20 7a9 9 0 0 0-15-2L2 8m0-5v5h5m-3 9a9 9 0 0 0 15 2l3-3m0 5v-5h-5M12 7v6m0 4h.01" /></>,
  video_library: <><rect x="6" y="6" width="15" height="15" rx="2" /><path d="M17 3H4a1 1 0 0 0-1 1v13m9-7 5 3-5 3Z" /></>,
  tune: <><path d="M3 6h3m4 0h11M3 12h11m4 0h3M3 18h5m4 0h9" /><circle cx="8" cy="6" r="2" /><circle cx="16" cy="12" r="2" /><circle cx="10" cy="18" r="2" /></>,
  close: <><path d="m6 6 12 12M18 6 6 18" /></>,
  send: <><path d="m22 2-7 20-4-9L2 9l20-7ZM11 13 22 2" /></>,
  refresh: <><path d="M20 7a8.5 8.5 0 1 0 .5 9M20 3v5h-5" /></>,
  done_all: <><path d="m2 12 4 4L17 5m-7 11 3 3L23 8" /></>,
  expand_more: <><path d="m6 9 6 6 6-6" /></>,
  manage_accounts: <><circle cx="9" cy="7" r="4" /><path d="M2 21v-2a7 7 0 0 1 10-6m7 0v2m0 6v2m-5-5h2m6 0h2m-8.5-3.5 1.4 1.4m4.2 4.2 1.4 1.4m0-7-1.4 1.4m-4.2 4.2-1.4 1.4" /><circle cx="19" cy="18" r="3" /></>,
  save: <><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h12l4 4v12a2 2 0 0 1-2 2Z" /><path d="M7 3v6h10V3M7 21v-8h10v8" /></>,
  movie: <><rect x="3" y="4" width="18" height="17" rx="2" /><path d="M3 9h18M7 4l3 5m4-5 3 5m-7 4 5 3-5 3Z" /></>,
  image: <><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="8" cy="8" r="1.5" /><path d="m21 15-6-6L3 21" /></>,
  person_add: <><circle cx="9" cy="7" r="4" /><path d="M2 21v-2a7 7 0 0 1 14 0v2m3-12v6m-3-3h6" /></>,
  campaign: <><path d="m3 9 17-6v16L3 13V9Zm4 5 2 7h4l-2-6M20 8h2v6h-2" /></>,
  play_arrow: <><path d="m8 5 12 7-12 7V5Z" fill="currentColor" stroke="none" /></>,
  pause: <><path d="M7 5v14M17 5v14" strokeWidth="4" /></>,
  volume_off: <><path d="M11 5 6 9H2v6h4l5 4V5Zm6 4 5 6m0-6-5 6" /></>,
  volume_up: <><path d="M11 5 6 9H2v6h4l5 4V5Zm5 3a6 6 0 0 1 0 8m3-11a10 10 0 0 1 0 14" /></>,
  fullscreen: <><path d="M8 3H3v5m13-5h5v5M3 16v5h5m8 0h5v-5" /></>,
  fullscreen_exit: <><path d="M3 8h5V3m8 0v5h5M8 21v-5H3m18 0h-5v5" /></>,
  arrow_back: <><path d="M20 12H4m6-6-6 6 6 6" /></>,
  arrow_forward: <><path d="M4 12h16m-6-6 6 6-6 6" /></>,
  chevron_left: <><path d="m15 6-6 6 6 6" /></>,
  chevron_right: <><path d="m9 6 6 6-6 6" /></>,
  grid_view: <><rect x="3" y="3" width="7" height="7" rx="1" /><rect x="14" y="3" width="7" height="7" rx="1" /><rect x="3" y="14" width="7" height="7" rx="1" /><rect x="14" y="14" width="7" height="7" rx="1" /></>,
  view_agenda: <><rect x="3" y="3" width="18" height="7" rx="1" /><rect x="3" y="14" width="18" height="7" rx="1" /></>,
  check: <><path d="m4 12 5 5L20 6" /></>,
  content_copy: <><rect x="8" y="8" width="13" height="13" rx="2" /><path d="M16 8V3H3v13h5" /></>,
  keyboard: <><rect x="2" y="5" width="20" height="14" rx="2" /><path d="M6 9h.01M10 9h.01M14 9h.01M18 9h.01M6 12h.01M10 12h.01M14 12h.01M18 12h.01M7 16h10" /></>,
  arrow_outward: <><path d="M6 18 18 6M6 6h12v12" /></>,
};

export default function Icon({ name, size = 20, className = "", ...props }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`icon ${className}`.trim()}
      aria-hidden={props["aria-label"] ? undefined : "true"}
      focusable="false"
      {...props}
    >
      {paths[name] || paths.auto_awesome}
    </svg>
  );
}
