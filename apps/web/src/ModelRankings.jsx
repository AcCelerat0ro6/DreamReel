import { useEffect, useState } from "react";

const sorts = [{ key: "hot", label: "综合热度" }, { key: "works", label: "作品数量" }, { key: "likes", label: "获赞数量" }];
const number = (value) => Number(value || 0).toLocaleString("zh-CN");

export default function ModelRankings({ request, onNavigate }) {
  const [sort, setSort] = useState("hot");
  const [page, setPage] = useState(0);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    setData(null);
    request(`/api/model-rankings?sort=${sort}&limit=20&offset=${page * 20}`)
      .then((result) => { if (!cancelled) setData(result); })
      .catch(() => { if (!cancelled) setError("榜单加载失败，请稍后重试。"); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [request, sort, page, revision]);

  return (
    <main className="model-rankings-page">
      <section className="ranking-header">
        <div>
          <p className="eyebrow">AIGC · 创作观察</p>
          <h1>模型榜单</h1>
          <p>从社区作品出发，发现正在被创作者使用的生成模型。</p>
        </div>
        <button className="ghost-button compact" disabled={loading} onClick={() => setRevision((value) => value + 1)}>
          <span className="material-symbols-outlined">refresh</span>刷新榜单
        </button>
      </section>
      <div className="ranking-toolbar" role="group" aria-label="榜单排序">
        {sorts.map((option) => (
          <button key={option.key} aria-pressed={sort === option.key} className={sort === option.key ? "active" : ""}
            onClick={() => { setSort(option.key); setPage(0); }}>{option.label}</button>
        ))}
      </div>
      <p className="ranking-note">累计统计已发布且填写模型名称的作品。热度 = 点赞 × 3 + 评论 × 5 + 收藏 × 4；这是社区创作榜单，不代表模型能力评测。互动数据会随统计更新。</p>
      <section className="ranking-content" aria-label="模型排名" aria-busy={loading}>
        {loading ? <p className="ranking-state" role="status">正在加载模型榜单…</p> : error ? (
          <div className="ranking-state" role="alert"><p>{error}</p><button className="ghost-button compact" onClick={() => setRevision((value) => value + 1)}>重新加载</button></div>
        ) : !data?.items?.length ? (
          <div className="ranking-state"><h2>{page ? "本页暂无模型" : "还没有模型上榜"}</h2><p>发布视频时填写模型名称，作品就会计入榜单。</p><button className="primary-button" onClick={() => onNavigate("/upload")}>发布作品</button></div>
        ) : (
          <div className="ranking-table-scroll">
            <table className="ranking-table">
              <caption className="sr-only">模型榜单，按{sorts.find((option) => option.key === sort).label}排序</caption>
              <thead><tr><th scope="col">排名</th><th scope="col">生成模型</th><th scope="col">作品</th><th scope="col">点赞</th><th scope="col">评论</th><th scope="col">收藏</th><th scope="col">热度</th></tr></thead>
              <tbody>{data.items.map((item) => (
                <tr key={item.model_name}>
                  <td><span className={`ranking-position ${item.rank <= 3 ? "top" : ""}`}>{item.rank.toString().padStart(2, "0")}</span></td>
                  <th scope="row">{item.model_name}</th>
                  <td>{number(item.video_count)}</td><td>{number(item.like_count)}</td><td>{number(item.comment_count)}</td><td>{number(item.favorite_count)}</td>
                  <td className="ranking-score">{number(item.hot_score)}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
      </section>
      <nav className="ranking-pagination" aria-label="榜单分页">
        <button className="ghost-button compact" disabled={loading || page === 0} onClick={() => setPage((value) => value - 1)}>上一页</button>
        <span>第 {page + 1} 页</span>
        <button className="ghost-button compact" disabled={loading || !data?.has_more || page >= 500} onClick={() => setPage((value) => value + 1)}>下一页</button>
      </nav>
    </main>
  );
}
