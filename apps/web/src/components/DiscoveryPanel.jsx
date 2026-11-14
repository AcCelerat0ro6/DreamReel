import { useEffect, useState } from 'react';
import Icon from './Icon';

export default function DiscoveryPanel({ request, items, index, onSelect, onNavigate }) {
  const [models, setModels] = useState(null);
  const [error, setError] = useState(false);
  useEffect(() => { let live = true; request('/api/model-rankings?sort=hot&limit=4&offset=0').then(data => { if (live) setModels(data.items || []); }).catch(() => { if (live) setError(true); }); return () => { live = false; }; }, [request]);
  return <aside className="discovery-panel">
    <section className="insight-card"><header><h2><Icon name="leaderboard" size={18}/>创作模型榜</h2><button onClick={() => onNavigate('/models')} aria-label="查看完整模型榜单"><Icon name="arrow_outward" size={18}/></button></header><p className="insight-subtitle">社区创作热度 · 持续更新</p><div className="mini-model-list">{models?.map((model, i) => <button key={model.model_name} onClick={() => onNavigate('/models')}><span className={`mini-rank rank-${i}`}>{String(i + 1).padStart(2, '0')}</span><span className={`model-monogram model-${i}`}>{model.model_name.slice(0, 1).toUpperCase()}</span><span className="mini-model-name"><strong>{model.model_name}</strong><small>{Number(model.video_count || 0).toLocaleString()} 部作品</small></span><span className="mini-heat"><Icon name="local_fire_department" size={13}/>{Number(model.hot_score || 0).toLocaleString()}</span></button>)}{!models && !error && <p className="muted small-text">正在发现热门模型…</p>}{(models?.length === 0 || error) && <p className="muted small-text">{error ? '暂时无法获取榜单' : '发布作品，让你的模型上榜。'}</p>}</div><button className="card-text-link" onClick={() => onNavigate('/models')}>探索全部模型 <Icon name="arrow_forward" size={14}/></button></section>
    <section className="inspiration-note"><div><span className="eyebrow">THE NEXT IDEA</span><Icon name="auto_awesome" size={21}/></div><h3>下一帧，由你想象。</h3><p>一个意想不到的场景，<br/>就可能是好故事的开始。</p><button onClick={() => onNavigate('/upload')}>把灵感变成作品 <Icon name="arrow_outward" size={16}/></button><span className="note-decoration" aria-hidden="true">✳</span></section>
    {items.length > 1 && <section className="queue-card"><header><h2>接下来看看</h2><span>{items.length} 部已加载</span></header>{items.map((item, i) => ({ item, i })).filter(entry => entry.i !== index).slice(0, 2).map(({ item, i }) => <button className="queue-item" key={item.entry_id || item.video_id} onClick={() => onSelect(i)}><span className="queue-thumb"><img src={item.cover_url || '/cover.svg'} alt=""/><Icon name="play_arrow" size={18}/></span><span><strong>{item.title}</strong><small>{item.model_name || 'AI 原创影像'}</small></span></button>)}</section>}
    <p className="discovery-footer">每一帧想象，都有回响。<span>DreamReel © 2026</span></p>
  </aside>;
}
