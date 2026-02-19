/**
 * @fileoverview Ban primitive color scales in Tailwind classes
 *
 * This rule prevents developers from using primitive color scales directly
 * in Tailwind className strings. All UI colors should use semantic tokens
 * defined in src/index.css (success, warning, error, info, accent, text-*, etc).
 *
 * Examples of incorrect code:
 *   className="bg-emerald-400"
 *   className="text-gray-500"
 *   className="border-red-500/20"
 *
 * Examples of correct code:
 *   className="bg-success"
 *   className="text-text-muted"
 *   className="border-error/20"
 */

/** @type {import('eslint').Rule.RuleModule} */
module.exports = {
  meta: {
    type: 'problem',
    docs: {
      description: 'Disallow primitive color scales in Tailwind classes',
      category: 'Best Practices',
      recommended: true,
    },
    messages: {
      primitiveColorScale:
        'Primitive color scale "{{scale}}" detected in "{{match}}". Use semantic tokens instead:\n' +
        '  \u2022 Status: success, warning, error, info\n' +
        '  \u2022 Accent: accent, accent-light\n' +
        '  \u2022 Text: text-primary, text-secondary, text-tertiary, text-muted, text-disabled\n' +
        '  \u2022 Surface: bg, surface, surface-light, surface-lighter, border\n' +
        '  \u2022 Overlay: overlay\n' +
        '  \u2022 Custom: define new tokens in src/index.css @theme',
    },
    schema: [],
  },

  create(context) {
    const primitiveScales = [
      'neutral',
      'gray',
      'red',
      'orange',
      'amber',
      'yellow',
      'lime',
      'green',
      'emerald',
      'teal',
      'cyan',
      'sky',
      'blue',
      'indigo',
      'violet',
      'purple',
      'fuchsia',
      'pink',
      'rose',
    ];

    const primitivePattern = new RegExp(
      `\\b(?:bg|text|border|from|via|to|ring|outline|decoration|divide|accent|caret|fill|stroke|shadow)-(?:${primitiveScales.join('|')})(?:-(?:50|100|200|300|400|500|600|700|800|900|950))?(?:\\/\\d+)?\\b`,
      'g'
    );

    function checkForPrimitiveColors(node, value) {
      const matches = value.match(primitivePattern);
      if (matches) {
        matches.forEach(match => {
          const scaleMatch = primitiveScales.find(scale => match.includes(scale));
          context.report({
            node,
            messageId: 'primitiveColorScale',
            data: {
              scale: scaleMatch,
              match: match,
            },
          });
        });
      }
    }

    return {
      JSXAttribute(node) {
        if (
          node.name.name === 'className' &&
          node.value &&
          node.value.type === 'Literal' &&
          typeof node.value.value === 'string'
        ) {
          checkForPrimitiveColors(node, node.value.value);
        }

        if (
          node.name.name === 'className' &&
          node.value &&
          node.value.type === 'JSXExpressionContainer' &&
          node.value.expression.type === 'TemplateLiteral'
        ) {
          node.value.expression.quasis.forEach(quasi => {
            checkForPrimitiveColors(node, quasi.value.raw);
          });
        }
      },

      CallExpression(node) {
        const functionName = node.callee.name;
        if (['clsx', 'classnames', 'cn', 'cva'].includes(functionName)) {
          node.arguments.forEach(arg => {
            if (arg.type === 'Literal' && typeof arg.value === 'string') {
              checkForPrimitiveColors(arg, arg.value);
            }
            if (arg.type === 'TemplateLiteral') {
              arg.quasis.forEach(quasi => {
                checkForPrimitiveColors(arg, quasi.value.raw);
              });
            }
          });
        }
      },
    };
  },
};
