#!/usr/bin/env python3
"""Collect public metadata and download videos whose reuse the operator authorized."""
import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone
import json
from pathlib import Path
import re
import subprocess
import time
import urllib.request
import urllib.parse

API = 'https://api.bilibili.com'
HEADERS = {'User-Agent': 'DreamReel-Catalog/1.0'}
EXTRAS = '''BV12pnJ6DE5J BV16En36HE47 BV1ftJ86PE5Z BV1bXn76pEHn
BV1EvnJ6CECD BV1xKnn6HEpi BV1Nznj6VEaJ BV1LAnJ6vE3J BV19mJ16iEzk
BV1Jgnj6AE63 BV1amn76dESP BV1brn36iEN2 BV15B8z61EbV BV1yXt16rEWJ
BV1SobN66E5g BV1n6AUzcENe BV1GCEx6yEDS BV1sbEn6UEED BV1praT6gEWp
BV1YoYi61EtJ BV1s5QwBGEsv BV1Qho6BkEQQ BV11mFLziEyP BV1RANn6bEAW
BV1iyVh6DE1B BV19iLv6bEKh BV1yxr9B8EjY BV15MRKByEXk BV1RW27BXEL8
BV1YAQPBtELc BV12mP9z9Ec1 BV1a3th65EQb BV1LCaA6pEyi BV1tNab65EaX'''.split()


def get(path):
    request = urllib.request.Request(API + path, headers=HEADERS)
    with urllib.request.urlopen(request, timeout=30) as response:
        data = json.load(response)
    if data.get('code') != 0:
        raise RuntimeError(f"Bilibili API returned {data.get('code')}")
    return data['data']


def classify(title):
    for words, category in [
        ('世一上|LPL|电竞', '电竞短剧'), ('科幻|宇宙|加加林|幸存|花园', '科幻'),
        ('聊斋|侠女|槐灯|大傩|成仙|唐山', '国风'), ('十九层|雾|都市传说|秘密|最后一镜', '悬疑'),
        ('楼下|年少|爷爷|生活|开饭|阿黄', '治愈'),
    ]:
        if re.search(words, title):
            return category
    return '创意短片'


def collect(output, count):
    series = get('/x/web-interface/view?bvid=BV12gpt6UER4')
    candidates = [e['bvid'] for s in series.get('ugc_season', {}).get('sections', [])
                  for e in s.get('episodes', []) if '世一上归位' in e['title']]
    candidates += EXTRAS
    records = []
    for bvid in dict.fromkeys(candidates):
        if len(records) >= count:
            break
        try:
            data = get('/x/web-interface/view?bvid=' + bvid)
            rights = data.get('rights', {})
            if data.get('state', 0) != 0 or rights.get('pay') or rights.get('ugc_pay') or rights.get('no_share'):
                print(f'Skipping unavailable/restricted: {bvid}', flush=True)
                continue
            if not 10 <= data['duration'] <= 1100:
                continue
            if data['stat'].get('view', 0) < 10000:
                continue
            description = data.get('desc', '')
            # Attribute a model only when the creator explicitly mentions it.
            model = next((label for pattern, label in [
                (r'Seedance\s*2|即梦.*2\.0', 'Seedance 2.0'), (r'Seedance', 'Seedance'),
                (r'即梦', '即梦 AI'), (r'可灵|Kling', '可灵 AI'), (r'海螺|Hailuo', '海螺 AI'),
                (r'\bSora\b', 'Sora'), (r'\bVeo\b', 'Veo'), (r'\bRunway\b', 'Runway'),
                (r'LibTV', 'LibTV')
            ] if re.search(pattern, description, re.I)), '')
            records.append({
                'bvid': bvid, 'title': data['title'], 'source_url': f'https://www.bilibili.com/video/{bvid}/',
                'source_author': data['owner']['name'], 'source_author_id': data['owner']['mid'],
                'source_published_at': data['pubdate'], 'source_stats': {
                    k: data['stat'].get(k, 0) for k in ('view', 'like', 'favorite', 'reply', 'share')
                }, 'duration': data['duration'], 'category': classify(data['title']), 'model_name': model,
                'checked_at': datetime.now(timezone.utc).isoformat(),
            })
            print(f'Collected {len(records)}/{count}: {bvid}', flush=True)
        except Exception as error:
            print(f'Could not collect {bvid}: {error}', flush=True)
        time.sleep(0.7)
    if len(records) < 30:
        raise SystemExit(f'Only {len(records)} available videos; need at least 30.')
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps({'batch': 'bilibili-ai-20261108', 'authorization': 'operator-confirmed-in-chat',
                                 'videos': records}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(f'Saved {len(records)} source records to {output}', flush=True)


def probe(path):
    result = subprocess.run(['ffprobe', '-v', 'error', '-show_streams', '-show_format', '-of', 'json', str(path)],
                            capture_output=True, text=True, check=True)
    data = json.loads(result.stdout)
    videos = [s for s in data['streams'] if s['codec_type'] == 'video']
    audios = [s for s in data['streams'] if s['codec_type'] == 'audio']
    if not videos or not audios or float(data['format'].get('duration', 0)) < 10:
        raise RuntimeError('Downloaded media is missing video/audio or too short.')
    return videos[0], audios[0], float(data['format']['duration'])


def download_one(index, entry, staging, media_dir, cover_dir, downloader):
    bvid = entry['bvid']
    destination = media_dir / f'{bvid}.mp4'
    cover = cover_dir / f'{bvid}.jpg'
    with (staging / f'{bvid}.download.log').open('a', encoding='utf-8') as log:
        if not destination.exists():
            subprocess.run([
                str(downloader), '--ignore-config', '--no-playlist', '--no-progress',
                '--socket-timeout', '25', '--retries', '2', '--fragment-retries', '2',
                '-f', 'bv*[height<=720][vcodec^=avc1]+ba[acodec^=mp4a]/b[height<=720][ext=mp4]/bv*[height<=720]+ba/b',
                '--merge-output-format', 'mp4', '-o', str(staging / f'{bvid}.%(ext)s'), entry['source_url'],
            ], check=True, timeout=900, stdout=log, stderr=subprocess.STDOUT)
            source = next(p for p in staging.glob(f'{bvid}.*') if p.suffix in ('.mp4', '.mkv', '.webm', '.flv'))
            video, audio, _ = probe(source)
            temporary = destination.with_suffix('.next.mp4')
            codecs = ['-c:v', 'copy'] if video['codec_name'] == 'h264' else ['-c:v', 'libx264', '-preset', 'fast', '-crf', '23', '-pix_fmt', 'yuv420p']
            codecs += ['-c:a', 'copy'] if audio['codec_name'] == 'aac' else ['-c:a', 'aac', '-b:a', '128k']
            subprocess.run(['ffmpeg', '-nostdin', '-hide_banner', '-loglevel', 'error', '-y', '-i', str(source),
                            '-map', '0:v:0', '-map', '0:a:0', *codecs, '-movflags', '+faststart', str(temporary)],
                           check=True, timeout=900, stdout=log, stderr=subprocess.STDOUT)
            temporary.replace(destination)
        video, audio, duration = probe(destination)
        if abs(duration - entry['duration']) > max(8, entry['duration'] * 0.05):
            raise RuntimeError('Media duration does not match the source record.')
        if not cover.exists():
            subprocess.run(['ffmpeg', '-nostdin', '-hide_banner', '-loglevel', 'error', '-y', '-ss', '1',
                            '-i', str(destination), '-frames:v', '1', '-vf', 'scale=960:-2', '-q:v', '3', str(cover)], check=True, timeout=60, stdout=log, stderr=subprocess.STDOUT)
    return index, dict(entry, media_url=f'/uploads/catalog/video/{bvid}.mp4', cover_url=f'/uploads/catalog/cover/{bvid}.jpg',
                       width=video['width'], height=video['height'], bytes=destination.stat().st_size)


def download(manifest, root, downloader):
    data = json.loads(manifest.read_text(encoding='utf-8'))
    staging = root / '.runtime/import/downloads'
    media_dir = root / 'apps/api/uploads/catalog/video'
    cover_dir = root / 'apps/api/uploads/catalog/cover'
    for path in (staging, media_dir, cover_dir):
        path.mkdir(parents=True, exist_ok=True)
    ready_by_index = {}
    with ThreadPoolExecutor(max_workers=3) as pool:
        tasks = {pool.submit(download_one, index, entry, staging, media_dir, cover_dir, downloader): entry
                 for index, entry in enumerate(data['videos'])}
        for task in as_completed(tasks):
            entry = tasks[task]
            try:
                index, record = task.result()
                ready_by_index[index] = record
                print(f'READY {len(ready_by_index)}/{len(data["videos"])}: {entry["bvid"]}', flush=True)
            except Exception as error:
                print(f'FAILED {entry["bvid"]}: {error}', flush=True)
            ready = [ready_by_index[index] for index in sorted(ready_by_index)]
            temporary = root / '.runtime/import/ready.next.json'
            temporary.write_text(json.dumps(dict(data, videos=ready), ensure_ascii=False, indent=2), encoding='utf-8')
            temporary.replace(root / '.runtime/import/ready.json')
    print(f'COMPLETE: {len(ready)}/{len(data["videos"])} videos ready', flush=True)
    if len(ready) < 30:
        raise SystemExit('Fewer than 30 verified videos; no database import has been performed.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['collect', 'download'])
    parser.add_argument('--manifest', type=Path, default=Path('data/bilibili-ai-20261108.json'))
    parser.add_argument('--count', type=int, default=40)
    parser.add_argument('--root', type=Path, default=Path.cwd())
    parser.add_argument('--downloader', type=Path)
    args = parser.parse_args()
    if args.action == 'collect':
        collect(args.manifest, args.count)
    else:
        if not args.downloader or not args.downloader.is_file():
            parser.error('--downloader must point to the yt-dlp executable')
        download(args.manifest, args.root.resolve(), args.downloader.resolve())
