-- Application migrations create the relational tables. This script enables
-- the spatial extension before the application installs its geometry triggers.
CREATE EXTENSION IF NOT EXISTS postgis;
