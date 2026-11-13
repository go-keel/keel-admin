import { describe, expect, it } from 'vitest';
import { submittableRoleIds } from './users';

describe('submittableRoleIds', () => {
  const assignable = [{ id: '30' }, { id: '31' }];

  it('submits only assignable roles: initial values come from the detail and carry invisible roles (disabled, sensitive) that the backend would reject as overreach', () => {
    // The target holds 30 (assignable) and 99 (outside the assignable range, not in the dropdown).
    expect(submittableRoleIds(['30', '99'], assignable)).toEqual(['30']);
  });

  it('invisible roles are not submitted, which is not removing them: the backend adds them back from the current roles it reads under the lock', () => {
    // Holds only one invisible role: the submitted set is empty and the backend pins it back; the frontend does not judge emptiness for it.
    expect(submittableRoleIds(['99'], assignable)).toEqual([]);
  });

  it('when the options hold everything (root gets the full set) nothing is lost and order is kept', () => {
    expect(submittableRoleIds(['31', '30'], assignable)).toEqual(['31', '30']);
  });

  it('options not yet loaded do not mean "nothing assignable": better to submit nothing than an overreaching id', () => {
    expect(submittableRoleIds(['30'], [])).toEqual([]);
  });
});
