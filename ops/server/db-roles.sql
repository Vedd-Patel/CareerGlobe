-- Separate database users (spec: Security and operations):
--   cg_crawler  writes companies, sources, jobs, crawl runs (crawler/)
--   cg_api      reads everything, writes user data, admin edits, classification (api/, nightly)
--   cg_backup   reads everything (pg_dump)
-- Run as the database owner after migrations, with passwords from the server's .env:
--   psql "$DATABASE_URL" -v crawler_pw="'...'" -v api_pw="'...'" -v backup_pw="'...'" -f ops/server/db-roles.sql
-- Re-running is safe: it resets passwords and grants.

SELECT 'CREATE ROLE cg_crawler LOGIN' WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'cg_crawler') \gexec
SELECT 'CREATE ROLE cg_api LOGIN' WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'cg_api') \gexec
SELECT 'CREATE ROLE cg_backup LOGIN' WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'cg_backup') \gexec

ALTER ROLE cg_crawler PASSWORD :crawler_pw;
ALTER ROLE cg_api PASSWORD :api_pw;
ALTER ROLE cg_backup PASSWORD :backup_pw;

GRANT CONNECT ON DATABASE careerglobe TO cg_crawler, cg_api, cg_backup;
GRANT USAGE ON SCHEMA public TO cg_crawler, cg_api, cg_backup;

-- Backups read everything.
GRANT SELECT ON ALL TABLES IN SCHEMA public TO cg_backup;
GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO cg_backup;

-- Crawler: the crawl tables only. It never sees users, profiles or resumes.
GRANT SELECT, INSERT, UPDATE ON companies, company_locations, careers_sources, jobs, crawl_runs TO cg_crawler;
GRANT SELECT, UPDATE ON crawl_triggers TO cg_crawler;
GRANT SELECT ON spatial_ref_sys TO cg_crawler;

-- API and nightly matcher: read the crawl tables, write the user tables.
GRANT SELECT ON ALL TABLES IN SCHEMA public TO cg_api;
GRANT INSERT, UPDATE, DELETE ON
  users, profiles, resumes, preferences, matches, applications, application_events, interview_rounds,
  saved_companies, notifications, push_tokens, consents, access_attempts, access_tickets_used, reports,
  removal_requests, category_rules, skill_synonyms, city_aliases, crawl_triggers
TO cg_api;
GRANT INSERT, UPDATE ON access_codes TO cg_api;
-- Admin console: add companies and fix failing sources. Classifier: job columns only.
GRANT INSERT, UPDATE ON companies, company_locations, careers_sources TO cg_api;
GRANT UPDATE (category, category_2, seniority, skills, classified_hash) ON jobs TO cg_api;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO cg_crawler, cg_api;

-- Tables added by later migrations get the same read access.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO cg_backup, cg_api;
