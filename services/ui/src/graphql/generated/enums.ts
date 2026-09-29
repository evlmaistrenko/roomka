/**
 * Failures arrive in the response's `errors`. Their `message` is prose;
 * `extensions` states the same failure in terms this schema defines.
 *
 * - `path` — the field that failed
 * - `extensions.code` — one of these values
 *
 * Fields built on this base fail the same way:
 *
 * ```graphql
 * query {
 *   droids(filter: { manufacturedBefore: "long ago" }) {
 *     name
 *   }
 * }
 * ```
 *
 * ```json
 * {
 *   "errors": [
 *     {
 *       "message": "manufacturedBefore is not a date",
 *       "path": ["droids"],
 *       "extensions": {
 *         "code": "INVALID_INPUT",
 *         "inputPath": ["filter", "manufacturedBefore"],
 *         "reason": "MALFORMED"
 *       }
 *     }
 *   ]
 * }
 * ```
 */
export type ErrorCode =
  | 'ACCESS_DENIED'
  | 'INTERNAL'
  /**
   * Validation stops at the first bad argument and throws. Where an argument is
   * at fault, `extensions.inputPath` names it, outermost first.
   */
  | 'INVALID_INPUT'
  /** A cap is full. */
  | 'LIMIT_REACHED'
  /**
   * Too many attempts too quickly. `extensions.retryAfterSeconds` says when the
   * next one will be let through.
   */
  | 'RATE_LIMITED'
  /** Call `stepUp`, then retry unchanged. */
  | 'STEP_UP_REQUIRED'
  | 'UNAUTHENTICATED';

/** Why an argument was rejected, in `extensions.reason`. */
export type InvalidInputReason =
  | 'MALFORMED'
  | 'OUT_OF_RANGE'
  | 'TAKEN'
  /** The view is stale. */
  | 'TARGET_MISSING'
  | 'TOO_GUESSABLE'
  | 'TOO_LONG'
  | 'TOO_SHORT';

export type Permission =
  | 'USER_MANAGE';

export type Role =
  /** Grants `USER_MANAGE`. */
  | 'ADMIN';

export type SessionSort =
  | 'CREATED_ASC'
  | 'CREATED_DESC'
  | 'ENDED_ASC'
  | 'ENDED_DESC'
  | 'LAST_USED_ASC'
  | 'LAST_USED_DESC';

export type UserSort =
  | 'CREATED_ASC'
  | 'CREATED_DESC'
  | 'RANK_ASC'
  | 'RANK_DESC'
  | 'USERNAME_ASC'
  | 'USERNAME_DESC';
