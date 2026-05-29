'use strict';
const express = require('express');
const { Pool } = require('pg');
const fs = require('fs');
const path = require('path');

const app = express();
app.use(express.urlencoded({ extended: false }));

// Load template once at startup
const tmpl = fs.readFileSync(path.join(__dirname, 'templates', 'index.html'), 'utf8');

// DB — lazy connect, server starts immediately without it
let pool = null;
let dbReady = false;

function initDb() {
  const url = process.env.DATABASE_URL;
  if (!url) {
    console.log('DATABASE_URL not set — running without database (set it and redeploy)');
    return;
  }
  pool = new Pool({ connectionString: url, ssl: { rejectUnauthorized: false } });

  (async () => {
    let attempts = 0;
    while (attempts < 30) {
      try {
        await pool.query(`CREATE TABLE IF NOT EXISTS waitlist (
          id         SERIAL PRIMARY KEY,
          email      TEXT UNIQUE NOT NULL,
          created_at TIMESTAMPTZ DEFAULT NOW()
        )`);
        dbReady = true;
        console.log('Database connected and ready');
        return;
      } catch (err) {
        attempts++;
        console.log(`DB not ready (${attempts}/30): ${err.message}`);
        await new Promise(r => setTimeout(r, 5000));
      }
    }
    console.error('DB connection failed after 30 retries');
  })();
}

async function signupCount() {
  if (!dbReady) return 0;
  const { rows } = await pool.query('SELECT COUNT(*) AS n FROM waitlist');
  return parseInt(rows[0].n, 10);
}

function render(tmplStr, data) {
  return tmplStr.replace(/\{\{(\w+)\}\}/g, (_, k) => {
    const v = data[k];
    return v == null ? '' : String(v);
  });
}

app.get('/health', (_req, res) => res.send('ok'));

app.get('/', async (_req, res) => {
  const count = await signupCount();
  res.send(render(tmpl, { Count: count }));
});

app.post('/join', async (req, res) => {
  const email = (req.body.email || '').trim();
  if (!email || !email.includes('@') || !email.includes('.')) {
    return res.send('<p class="mt-3 text-red-400 text-sm">Please enter a valid email address.</p>');
  }
  if (!dbReady) {
    return res.send('<p class="mt-3 text-amber-400 text-sm">Database is warming up — try again in a moment.</p>');
  }
  try {
    await pool.query(
      'INSERT INTO waitlist (email) VALUES ($1) ON CONFLICT (email) DO NOTHING',
      [email]
    );
  } catch (err) {
    console.error('Insert error:', err.message);
    return res.send('<p class="mt-3 text-red-400 text-sm">Something went wrong. Try again.</p>');
  }
  const n = await signupCount();
  res.send(`<div class="mt-4 py-4 px-6 bg-emerald-500/10 border border-emerald-500/30 rounded-xl text-center">
  <p class="text-emerald-300 font-semibold text-lg">You're on the list! 🎉</p>
  <p class="text-slate-400 text-sm mt-1">You're one of <strong class="text-white">${n}</strong> people waiting for early access.</p>
</div>`);
});

app.get('/count', async (_req, res) => {
  const n = await signupCount();
  res.send(`<span hx-get="/count" hx-trigger="every 8s" hx-swap="outerHTML" class="font-bold text-emerald-400">${n}</span>`);
});

const port = process.env.PORT || 3000;
app.listen(port, () => console.log(`verdant listening on :${port}`));

initDb();
