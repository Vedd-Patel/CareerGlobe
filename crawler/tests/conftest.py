import pytest


# anyio's pytest plugin (installed with httpx) runs async tests; asyncio only.
@pytest.fixture
def anyio_backend():
    return "asyncio"
