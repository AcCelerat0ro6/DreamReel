export function filterVideos(items, query = '', model = '') {
  const needle = query.trim().toLocaleLowerCase();
  return items.map((item, index) => ({ item, index })).filter(({ item }) => (!model || item.model_name === model) && (!needle || [item.title, item.description, item.author, item.model_name, item.ai_style_tag, item.model_params].some(value => String(value || '').toLocaleLowerCase().includes(needle))));
}

export function sharedVideoIndex(items, search) {
  const id = new URLSearchParams(search).get('video');
  return id ? items.findIndex(item => String(item.video_id) === id) : -1;
}

export async function resolveSharedVideo(data, search, request, signal) {
  const id = new URLSearchParams(search).get('video');
  if (!id || !/^[1-9]\d*$/.test(id) || (data.items || []).some(item => String(item.video_id) === id)) return data;
  try {
    const video = await request(`/api/videos/${id}`, { signal });
    let author = {};
    try { author = await request(`/api/users/${video.author_id}`, { signal }); } catch (error) { if (signal?.aborted) throw error; }
    return { ...data, items: [{ ...video, video_id: video.id, author_nickname: author.nickname, author_avatar_url: author.avatar_url }, ...(data.items || [])] };
  } catch (error) {
    if (signal?.aborted) throw error;
    return { ...data, shareError: '分享的作品暂时不可用，先看看社区里的其他灵感。' };
  }
}
