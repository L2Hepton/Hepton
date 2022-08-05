# The builtin contracts of Hepton

Contracts under this `builtin` directory are all builtin contracts of Hepton, which will be allocated at genesis block (or at some hard-fork block, setup by setCode, not by deployment transactions). The builtin contracts have no creator. 

## genesis builtin contracts

## built or test

All operation should be done by `yarn` scripts, it's configured with some necessary environment variable.

```shell
yarn test
yarn compile
yarn clean
```
