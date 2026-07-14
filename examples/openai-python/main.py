"""
Minimal OpenAI SDK → TokenGuard integration.

pip install openai
export OPENAI_API_KEY=... TOKENGUARD_API_KEY=... TOKENGUARD_BASE_URL=https://....onrender.com
"""

import os
from openai import OpenAI

base = os.environ["TOKENGUARD_BASE_URL"].rstrip("/")
client = OpenAI(
    api_key=os.environ["OPENAI_API_KEY"],
    base_url=f"{base}/v1",
    default_headers={
        "X-TokenGuard-API-Key": os.environ["TOKENGUARD_API_KEY"],
        "X-TokenGuard-Provider": os.environ.get("TOKENGUARD_PROVIDER", "openai"),
        "X-TokenGuard-Session-ID": os.environ.get("TOKENGUARD_SESSION_ID", "example-python"),
    },
)

completion = client.chat.completions.create(
    model=os.environ.get("TOKENGUARD_MODEL", "gpt-4o-mini"),
    messages=[{"role": "user", "content": "Say hello in one short sentence."}],
    max_tokens=64,
)
print(completion.choices[0].message.content)
