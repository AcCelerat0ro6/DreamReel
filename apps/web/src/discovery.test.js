import test from 'node:test';
import assert from 'node:assert/strict';
import { filterVideos, sharedVideoIndex, resolveSharedVideo } from './discovery.js';

const items = [{ video_id: 7, title: 'Moon', author: 'Alice', model_name: 'Sora', model_params: 'cinematic light' }, { video_id: 8, title: '海底', author: 'Bob', model_name: '可灵', ai_style_tag: '水墨' }];
test('搜索涵盖模型、作者和创作参数，保留原索引用于定位播放', () => {
  assert.deepEqual(filterVideos(items, ' ALICE '), [{ item: items[0], index: 0 }]);
  assert.equal(filterVideos(items, 'cinematic', 'Sora').length, 1);
  assert.equal(filterVideos(items, '水墨')[0].index, 1);
  assert.equal(filterVideos(items, 'Moon', '可灵').length, 0);
});
test('分享命中当前页时不会重复请求', async () => {
  const data = { items, next_cursor: 'x', has_more: true };
  assert.equal(await resolveSharedVideo(data, '?video=8', () => { throw Error('unexpected'); }), data);
  assert.equal(sharedVideoIndex(items, '?video=8'), 1);
  assert.equal(sharedVideoIndex(items, '?video=999'), -1);
});
test('首屏外的分享作品通过公开详情补取，保留分页及作者资料', async () => {
  const calls = [];
  const result = await resolveSharedVideo({ items, next_cursor: 'x', has_more: true }, '?video=9', async path => { calls.push(path); return path === '/api/videos/9' ? { id: 9, author_id: 2, title: 'Shared' } : { nickname: 'Creator', avatar_url: '/avatar.svg' }; });
  assert.deepEqual(calls, ['/api/videos/9', '/api/users/2']);
  assert.equal(result.items[0].video_id, 9);
  assert.equal(result.items[0].author_nickname, 'Creator');
  assert.equal(result.next_cursor, 'x');
  assert.equal(result.has_more, true);
});
test('失效分享给出提示，非法参数不请求，取消不会覆盖页面', async () => {
  const data = { items };
  const missing = await resolveSharedVideo(data, '?video=999', async () => { throw Error('404'); });
  assert.equal(missing.items, items); assert.ok(missing.shareError);
  assert.equal(await resolveSharedVideo(data, '?video=../x', () => { throw Error('unexpected'); }), data);
  const controller = new AbortController(); controller.abort();
  await assert.rejects(resolveSharedVideo(data, '?video=9', async () => { throw new Error('aborted'); }, controller.signal), /aborted/);
});
