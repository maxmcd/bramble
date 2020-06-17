def build_python_package(*args, **kwargs):
    pass


build_python_package(
    pname="numpy",
    version="1.18.5",
    nativeBuildInputs=["gfortran", "pytest", "cython", "setuptoolsBuildHook"],
    buildInputs=["blas", "lapack"],
    checkPhase="""
    echo hi
    """,
)
