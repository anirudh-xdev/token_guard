/**
 * Minimal OpenAI SDK → TokenGuard integration.
 *
 * npm i openai
 * set OPENAI_API_KEY, TOKENGUARD_API_KEY, TOKENGUARD_BASE_URL
 */
import OpenAI from "openai";

const client = new OpenAI({
  apiKey: process.env.OPENAI_API_KEY,
  baseURL: `${process.env.TOKENGUARD_BASE_URL.replace(/\/$/, "")}/v1`,
  defaultHeaders: {
    "X-TokenGuard-API-Key": process.env.TOKENGUARD_API_KEY,
    "X-TokenGuard-Provider": process.env.TOKENGUARD_PROVIDER || "openai",
    "X-TokenGuard-Session-ID": process.env.TOKENGUARD_SESSION_ID || "example-node",
  },
});

const completion = await client.chat.completions.create({
  model: process.env.TOKENGUARD_MODEL || "gpt-4o-mini",
  messages: [{ role: "user", content: "Say hello in one short sentence." }],
  max_tokens: 64,
});

console.log(completion.choices[0]?.message?.content);
