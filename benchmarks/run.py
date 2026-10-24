#!/usr/bin/env python3
"""Reproducible comparison; destructive reset is confined to counters-sis2-bench."""
import argparse
import gzip
import json
import os
import pathlib
import platform
import subprocess
import threading
import time

ROOT = pathlib.Path(__file__).resolve().parents[1]
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'benchmarks/compose.yaml'), '-p', 'counters-sis2-bench']


def command(args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, text=True, **kwargs)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--duration', default='60s')
    parser.add_argument('--warmup', default='10s')
    parser.add_argument('--runs', type=int, default=3)
    parser.add_argument('--concurrency', default='1,10,50')
    parser.add_argument('--operations', default='read,views,likes')
    parser.add_argument('--output', default='benchmarks/results/local')
    parser.add_argument('--binary', default='/tmp/highload-counters-bench')
    args = parser.parse_args()
    output = ROOT / args.output
    output.mkdir(parents=True, exist_ok=False)
    command(['go', 'build', '-o', args.binary, './cmd/bench'])
    command(COMPOSE + ['up', '--build', '-d'])
    ids = command(COMPOSE + ['ps', '-q', 'api', 'worker', 'postgres', 'redis'], capture_output=True).stdout.split()
    environment = {
        'arguments': vars(args), 'machine': platform.uname()._asdict(), 'logicalCPUs': os.cpu_count(),
        'cpuinfo': pathlib.Path('/proc/cpuinfo').read_text().split('\n\n')[0],
        'meminfo': pathlib.Path('/proc/meminfo').read_text(),
        'docker': command(['docker', 'version', '--format', '{{json .}}'], capture_output=True).stdout,
        'containers': json.loads(command(['docker', 'inspect', *ids], capture_output=True).stdout),
        'go': command(['go', 'version'], capture_output=True).stdout,
        'gitHead': command(['git', 'rev-parse', 'HEAD'], capture_output=True).stdout.strip(),
        'gitStatus': command(['git', 'status', '--short'], capture_output=True).stdout,
        'dependencies': (ROOT / 'go.mod').read_text(),
        'transport': {'REST': 'HTTP/1.1 JSON', 'gRPC': 'HTTP/2 Protobuf', 'TLS': False, 'compression': False, 'retries': 0},
    }
    (output / 'environment.json').write_text(json.dumps(environment, indent=2))
    command([args.binary, '-payload'], stdout=(output / 'payload.json').open('w'))
    summaries = []
    for op in args.operations.split(','):
        for concurrency in map(int, args.concurrency.split(',')):
            for run in range(1, args.runs + 1):
                # Alternate order to reduce systematic ordering bias.
                for protocol in (['rest', 'grpc'] if run % 2 else ['grpc', 'rest']):
                    command(COMPOSE + ['stop', 'worker'], stdout=subprocess.DEVNULL)
                    sql = 'TRUNCATE likes, counter_flush_log, post_counters, posts; INSERT INTO posts SELECT n,\'Post \'||n FROM generate_series(1,100)n; INSERT INTO post_counters(post_id) SELECT id FROM posts;'
                    command(COMPOSE + ['exec', '-T', 'postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-U', 'bench', '-d', 'counters_bench', '-c', sql], stdout=subprocess.DEVNULL)
                    # This Redis belongs ONLY to this disposable benchmark project.
                    command(COMPOSE + ['exec', '-T', 'redis', 'redis-cli', 'FLUSHDB'], stdout=subprocess.DEVNULL)
                    command(COMPOSE + ['start', 'worker'], stdout=subprocess.DEVNULL)
                    label = f'{op}-c{concurrency}-{protocol}-r{run}'
                    address = 'http://localhost:18080' if protocol == 'rest' else 'localhost:19090'
                    cmd = [args.binary, '-protocol', protocol, '-address', address, '-op', op,
                           '-concurrency', str(concurrency), '-duration', args.duration, '-warmup', args.warmup, '-retries', '0']
                    stopped = threading.Event()
                    stats = []
                    def sample_resources():
                        while not stopped.is_set():
                            result = subprocess.run(['docker', 'stats', '--no-stream', '--format', '{{json .}}', *ids], text=True, capture_output=True, timeout=15)
                            stats.append({'time': time.time(), 'exitCode': result.returncode, 'data': result.stdout, 'error': result.stderr})
                            stopped.wait(.2)
                    thread = threading.Thread(target=sample_resources)
                    thread.start()
                    try:
                        result = command(cmd, capture_output=True)
                    finally:
                        stopped.set()
                        thread.join()
                    with gzip.open(output / f'{label}.json.gz', 'wt') as stream:
                        stream.write(result.stdout)
                    (output / f'{label}-resources.json').write_text(json.dumps(stats))
                    data = json.loads(result.stdout)
                    data.pop('samples', None)
                    data.update({'run': run, 'command': cmd, 'protocol': protocol, 'operation': op})
                    summaries.append(data)
                    (output / 'summary.json').write_text(json.dumps(summaries, indent=2))
                    if op == 'freshness':
                        delays = sorted(data['acceptedToVisibleMs'])
                        print(label, 'probes', len(delays), 'max ms', round(delays[-1]) if delays else None, 'violations>2s', data['violationsOver2s'], flush=True)
                    else:
                        print(label, 'RPS', round(data['rps']), 'p95', round(data['p95ms'], 2), 'errors', data['errors'], flush=True)
    print('Results:', output)


if __name__ == '__main__':
    main()
