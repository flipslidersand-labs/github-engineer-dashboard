from app.cache import SQLiteCache


class FakeClock:
    def __init__(self, t: float = 1000.0) -> None:
        self.t = t

    def __call__(self) -> float:
        return self.t


def test_set_get_roundtrip(tmp_path):
    cache = SQLiteCache(str(tmp_path / "c.db"), ttl_seconds=300)
    cache.set("k", {"a": 1, "b": [1, 2]})
    assert cache.get("k") == {"a": 1, "b": [1, 2]}


def test_missing_key_returns_none(tmp_path):
    cache = SQLiteCache(str(tmp_path / "c.db"))
    assert cache.get("nope") is None


def test_expiry(tmp_path):
    clock = FakeClock(1000.0)
    cache = SQLiteCache(str(tmp_path / "c.db"), ttl_seconds=300, clock=clock)
    cache.set("k", {"v": 1})

    clock.t = 1000.0 + 299  # still fresh
    assert cache.get("k") == {"v": 1}

    clock.t = 1000.0 + 301  # expired
    assert cache.get("k") is None


def test_overwrite_updates_value_and_ttl(tmp_path):
    clock = FakeClock(1000.0)
    cache = SQLiteCache(str(tmp_path / "c.db"), ttl_seconds=100, clock=clock)
    cache.set("k", {"v": 1})
    clock.t = 1050.0
    cache.set("k", {"v": 2})  # refreshes expiry to 1150
    clock.t = 1120.0
    assert cache.get("k") == {"v": 2}


def _row_count(cache: SQLiteCache) -> int:
    return cache._conn.execute("SELECT COUNT(*) FROM cache").fetchone()[0]


def test_set_purges_expired_rows_without_rereading_key(tmp_path):
    clock = FakeClock(1000.0)
    cache = SQLiteCache(str(tmp_path / "c.db"), ttl_seconds=100, clock=clock)
    for i in range(5):
        cache.set(f"old{i}", {"i": i})
    assert _row_count(cache) == 5

    clock.t = 1000.0 + 101 + 3600  # past TTL and past the purge interval
    cache.set("new", {"v": 1})

    assert _row_count(cache) == 1
    assert cache.get("new") == {"v": 1}


def test_lock_for_dict_is_bounded(tmp_path):
    import gc

    cache = SQLiteCache(str(tmp_path / "c.db"))
    for i in range(100):
        with cache.lock_for(f"k{i}"):
            pass
    gc.collect()
    assert len(cache._key_locks) == 0


def test_lock_for_same_key_returns_same_lock_while_held(tmp_path):
    cache = SQLiteCache(str(tmp_path / "c.db"))
    a = cache.lock_for("k")
    assert cache.lock_for("k") is a
