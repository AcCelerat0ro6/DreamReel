import re
import html

with open('xiaolin_raw.html', 'r', encoding='utf-8') as f:
    raw = f.read()

start_needle = 'theme-default-content'
start = raw.find(start_needle)
if start != -1:
    start = raw.rfind('<', 0, start)
    # find end of article
    end = raw.find('<footer class="page-meta"', start)
    if end == -1:
        end = raw.find('</main>', start)
    content = raw[start:end]
else:
    content = raw

with open('article_raw_fragment.html', 'w', encoding='utf-8') as f:
    f.write(content)

print('Extracted fragment size:', len(content))
