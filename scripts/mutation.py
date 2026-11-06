import difflib
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile


def execute_mutant():
    original = Path(os.environ['MUTATE_ORIGINAL'])
    changed = Path(os.environ['MUTATE_CHANGED'])
    contents = original.read_bytes()
    timeout = int(os.environ['MUTATE_TIMEOUT'])
    try:
        original.write_bytes(changed.read_bytes())
        with tempfile.TemporaryDirectory(prefix='tripgo-mutant-') as folder:
            binary = str(Path(folder) / 'mutant.test')
            build = subprocess.run(
                ['go', 'test', '-c', '-o', binary, os.environ['MUTATE_PACKAGE']],
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=120,
            )
            if build.returncode != 0:
                return 2
            test = subprocess.run(
                [binary, '-test.count=1', f'-test.timeout={timeout}s'],
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout + 5,
            )
            if b'test timed out after' in test.stdout:
                return 3
            if test.returncode not in (0, 1, 2):
                return 3
            return 1 if test.returncode == 0 else 0
    except subprocess.TimeoutExpired:
        return 3
    finally:
        original.write_bytes(contents)


def run_mutations():
    phase = sys.argv[1] if len(sys.argv) > 1 else 'latest'
    if phase not in ('before', 'after', 'latest'):
        raise SystemExit('Expected report name: before, after or latest')
    root = Path(__file__).resolve().parent.parent
    tool = shutil.which(os.environ.get('GO_MUTESTING', 'go-mutesting'))
    if tool is None:
        raise SystemExit('go-mutesting is not installed; see reports/mutation/README.md')
    build_info = subprocess.run(['go', 'version', '-m', tool], check=True, capture_output=True, text=True).stdout
    version = 'v0.0.0-20261002081621-9ce5f731e321'
    if f'\tmod\tgithub.com/avito-tech/go-mutesting\t{version}\t' not in build_info:
        raise SystemExit(f'Expected go-mutesting {version}; see reports/mutation/README.md')
    output = root / 'reports' / 'mutation'
    output.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env['GOFLAGS'] = env.get('GOFLAGS', '') + ' -race -buildvcs=false'
    env['GOWORK'] = 'off'
    with tempfile.TemporaryDirectory(prefix='tripgo-mutation-') as folder:
        snapshot = Path(folder)
        for filename in ('go.mod', 'go.sum'):
            shutil.copy2(root / filename, snapshot / filename)
        shutil.copytree(root / 'internal', snapshot / 'internal')
        sources = {
            str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest()
            for package in ('domain', 'usecase')
            for path in sorted((root / 'internal' / package).rglob('*.go'))
        }
        command = [
            tool, '--config', str(root / '.go-mutesting.yml'),
            '--exec', f'{sys.executable} {Path(__file__).resolve()} exec',
            '--exec-timeout', '30', 'internal/domain/...', 'internal/usecase/...',
        ]
        log = output / f'{phase}.log'
        print(f'Running mutation tests with -race; output: {log}', flush=True)
        with log.open('w') as stream:
            result = subprocess.run(command, cwd=snapshot, env=env, stdout=stream, stderr=subprocess.STDOUT)
        report_path = snapshot / 'report.json'
        if result.returncode != 0 or not report_path.exists():
            raise SystemExit(f'Mutation run failed: see {log}')
        report = json.loads(report_path.read_text())
        for status in ('escaped', 'killed', 'errored', 'timeouted'):
            for mutant in report.get(status) or []:
                details = mutant['mutator']
                old = details['originalSourceCode'].splitlines(keepends=True)
                new = details['mutatedSourceCode'].splitlines(keepends=True)
                mutant['diff'] = ''.join(difflib.unified_diff(old, new, fromfile='Original', tofile='Mutated'))
                changes = difflib.SequenceMatcher(a=old, b=new).get_opcodes()
                details['originalStartLine'] = next((i + 1 for tag, i, _, _, _ in changes if tag != 'equal'), 0)
        (output / f'{phase}.json').write_text(json.dumps(report, indent=2) + '\n')
        metadata = {'sources_sha256': sources, 'go_flags': env['GOFLAGS'].strip(), 'tool_module': 'github.com/avito-tech/go-mutesting', 'tool_version': 'v0.0.0-20261002081621-9ce5f731e321'}
        (output / f'{phase}-metadata.json').write_text(json.dumps(metadata, indent=2) + '\n')
        print(json.dumps(report['stats'], indent=2))
        if report['stats']['errorCount'] or report['stats']['timeOutCount']:
            raise SystemExit('Mutation run has execution errors or timeouts; inspect the report')


if __name__ == '__main__':
    if len(sys.argv) > 1 and sys.argv[1] == 'exec':
        sys.exit(execute_mutant())
    run_mutations()
